// Package knowledge — KnowledgeIndexer index tài liệu .md chuẩn RAG
// (frontmatter theo docs/RAG_STANDARD.md của các component) vào bảng
// rag_documents của TiBrain, chunk theo heading H2 vào rag_vector_index.
package knowledge

import (
	"crypto/md5"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// frontMatter chứa các trường frontmatter đã parse từ file .md (chuẩn RAG).
type frontMatter struct {
	Title       string
	Description string
	Category    string
	Tags        string // comma-separated
	RagID       string
	Version     string
	LastUpdated string
}

// chunk là một đoạn nội dung theo heading H2, lưu vào rag_vector_index.
type chunk struct {
	ChunkID string // "<ragID>-c<N>"
	Text    string
	Order   int
}

// KnowledgeIndexer handles knowledge indexing
type KnowledgeIndexer struct {
	hub hubConn
}

// hubConn là interface thu hẹp mà indexer cần — giúp test dùng fake
// implementation thay vì mở DB thật.
type hubConn interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

// KnowledgeIndexOptions configures indexing
type KnowledgeIndexOptions struct {
	Sources []KnowledgeIndexSource
}

// KnowledgeIndexSource represents a source to index
type KnowledgeIndexSource struct {
	Path        string
	Category    string
	Description string
}

// KnowledgeIndexResult represents indexing results
type KnowledgeIndexResult struct {
	Indexed int // tổng số chunk mới ghi (hoặc 1/doc nếu đếm theo doc)
	Skipped int // file skip: chưa chuẩn RAG hoặc content không đổi
	Errors  []string
	Sources int
}

// NewKnowledgeIndexer creates a new knowledge indexer.
// Giữ signature cũ: nhận *db.Hub. Truyền nil sẽ tạo indexer không dùng
// được (Index trả error) — không panic.
func NewKnowledgeIndexer(hub hubOwner) *KnowledgeIndexer {
	if hub == nil {
		return &KnowledgeIndexer{hub: nil}
	}
	return &KnowledgeIndexer{hub: hub.DB()}
}

// hubOwner thu hẹp *db.Hub: chỉ cần method DB() *sql.DB.
type hubOwner interface {
	DB() *sql.DB
}

// Index scans các source path (thư mục hoặc file .md), parse frontmatter
// chuẩn RAG, upsert vào rag_documents theo rag_id và chunk theo H2 vào
// rag_vector_index. File không đổi (cùng content_hash) được skip.
func (k *KnowledgeIndexer) Index(opts KnowledgeIndexOptions) (*KnowledgeIndexResult, error) {
	result := &KnowledgeIndexResult{
		Errors:  []string{},
		Sources: len(opts.Sources),
	}

	if k == nil || k.hub == nil {
		return nil, fmt.Errorf("knowledge: indexer chưa được khởi tạo với DB hub")
	}

	for _, src := range opts.Sources {
		if src.Path == "" {
			result.Errors = append(result.Errors, "source path rỗng, bỏ qua")
			continue
		}
		files, err := collectMarkdownFiles(src.Path)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("source %s: %v", src.Path, err))
			continue
		}
		for _, file := range files {
			nChunks, skip, err := k.indexFile(file, src)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", file, err))
				continue
			}
			if skip {
				result.Skipped++
			} else {
				result.Indexed += nChunks
			}
		}
	}
	return result, nil
}

// collectMarkdownFiles trả về danh sách file .md: nếu path là file → chính nó;
// nếu là thư mục → walk đệ quy.
func collectMarkdownFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("không truy cập được path: %w", err)
	}
	if !info.IsDir() {
		if strings.HasSuffix(strings.ToLower(path), ".md") {
			return []string{path}, nil
		}
		return nil, nil // file không phải .md — bỏ qua
	}
	var files []string
	err = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() && strings.HasSuffix(strings.ToLower(p), ".md") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", path, err)
	}
	return files, nil
}

// indexFile xử lý 1 file: đọc, parse frontmatter, tính hash, upsert
// rag_documents, chunk theo H2 và ghi rag_vector_index.
// Trả về (số chunk, skipped, error).
func (k *KnowledgeIndexer) indexFile(path string, src KnowledgeIndexSource) (nChunks int, skipped bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false, fmt.Errorf("đọc file: %w", err)
	}
	text := string(raw)

	fm, body, hasFM := parseFrontMatter(text)
	if !hasFM {
		// File không theo chuẩn RAG (thiếu frontmatter hoặc thiếu rag_id)
		// → skip, không phải lỗi: file chưa được chuẩn hóa.
		return 0, true, nil
	}

	// Fallback title từ H1 nếu frontmatter thiếu title.
	title := fm.Title
	if title == "" {
		title = firstH1(body)
	}
	if title == "" {
		title = filepath.Base(path)
	}

	// Category: frontmatter > source category > derive từ path.
	category := fm.Category
	if category == "" {
		category = src.Category
	}
	if category == "" {
		category = categoryFromPath(path)
	}

	contentHash := fmt.Sprintf("%x", md5.Sum(raw))
	now := time.Now().Unix()

	// Upsert theo rag_id.
	var existingHash string
	err = k.hub.QueryRow(
		`SELECT content_hash FROM rag_documents WHERE id = ?`, fm.RagID,
	).Scan(&existingHash)
	switch {
	case err == nil:
		if existingHash == contentHash {
			// Content không đổi → cập nhật last_indexed, không re-index.
			if _, uerr := k.hub.Exec(
				`UPDATE rag_documents SET last_indexed = ?, indexing_status = 'indexed' WHERE id = ?`,
				now, fm.RagID); uerr != nil {
				return 0, true, fmt.Errorf("update last_indexed: %w", uerr)
			}
			return 0, true, nil
		}
		// Content đổi → UPDATE doc + thay chunk.
		if _, uerr := k.hub.Exec(`
			UPDATE rag_documents SET
				title = ?, content = ?, path = ?, category = ?, tags = ?,
				updated_at = ?, status = 'active', content_hash = ?,
				file_size = ?, last_indexed = ?, indexing_status = 'indexed', metadata = ?
			WHERE id = ?`,
			title, body, path, category, fm.Tags,
			now, contentHash, len(raw), now, fm.Description, fm.RagID); uerr != nil {
			return 0, false, fmt.Errorf("update rag_documents: %w", uerr)
		}
		if _, derr := k.hub.Exec(
			`DELETE FROM rag_vector_index WHERE document_id = ?`, fm.RagID); derr != nil {
			return 0, false, fmt.Errorf("xóa chunk cũ: %w", derr)
		}
	case err == sql.ErrNoRows:
		if _, ierr := k.hub.Exec(`
			INSERT INTO rag_documents
				(id, title, content, path, category, tags, created_at, updated_at,
				 status, metadata, content_hash, file_size, last_indexed, indexing_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?, ?, ?, 'indexed')`,
			fm.RagID, title, body, path, category, fm.Tags, now, now,
			fm.Description, contentHash, len(raw), now); ierr != nil {
			return 0, false, fmt.Errorf("insert rag_documents: %w", ierr)
		}
	default:
		return 0, false, fmt.Errorf("truy vấn rag_documents: %w", err)
	}

	// Chunk theo heading H2 → rag_vector_index.
	chunksList := chunkByH2(body, fm.RagID)
	for _, c := range chunksList {
		if _, ierr := k.hub.Exec(`
			INSERT INTO rag_vector_index (id, document_id, chunk_id, chunk_text, chunk_order, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("%s-v%d", c.ChunkID, now), fm.RagID, c.ChunkID, c.Text, c.Order, now); ierr != nil {
			return len(chunksList), false, fmt.Errorf("insert chunk %s: %w", c.ChunkID, ierr)
		}
	}
	return len(chunksList), false, nil
}

// parseFrontMatter tách YAML frontmatter (--- ... ---) ở đầu file.
// Trả về frontMatter, body (phần sau ---), và ok=true nếu frontmatter
// hợp lệ theo chuẩn RAG (có rag_id).
func parseFrontMatter(text string) (fm frontMatter, body string, ok bool) {
	norm := strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(norm, "---\n") {
		return fm, text, false
	}
	end := strings.Index(norm[4:], "\n---")
	if end < 0 {
		return fm, text, false
	}
	fmBlock := norm[4 : 4+end]
	body = strings.TrimLeft(norm[4+end+4:], "\n")

	for _, line := range strings.Split(fmBlock, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, `"'`)
		switch key {
		case "title":
			fm.Title = val
		case "description":
			fm.Description = val
		case "category":
			fm.Category = val
		case "tags":
			fm.Tags = normalizeTags(val)
		case "rag_id":
			fm.RagID = val
		case "version":
			fm.Version = val
		case "last_updated":
			fm.LastUpdated = val
		}
	}
	if fm.RagID == "" {
		return fm, body, false // frontmatter có nhưng thiếu rag_id → chưa chuẩn
	}
	return fm, body, true
}

// normalizeTags: "[a, b] | a,b | [a b]" → "a,b"
func normalizeTags(val string) string {
	val = strings.TrimSpace(val)
	val = strings.TrimPrefix(val, "[")
	val = strings.TrimSuffix(val, "]")
	parts := strings.FieldsFunc(val, func(r rune) bool { return r == ',' || r == ' ' })
	var clean []string
	for _, p := range parts {
		if p != "" {
			clean = append(clean, p)
		}
	}
	return strings.Join(clean, ",")
}

// firstH1 trả về text của heading H1 đầu tiên, rỗng nếu không có.
func firstH1(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(line[2:])
		}
	}
	return ""
}

var h2Re = regexp.MustCompile(`(?m)^##\s+`)

// chunkByH2 chia body theo heading H2 ("## "). Nội dung trước H2 đầu
// (H1 + intro) thành chunk 0 (nếu không rỗng); mỗi H2 là 1 chunk kế tiếp.
// Tiêu đề H2 được giữ trong chunk để chunk self-contained theo chuẩn
// docs/RAG_STANDARD.md mục 4.
func chunkByH2(body string, ragID string) []chunk {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	locs := h2Re.FindAllStringIndex(body, -1)
	if len(locs) == 0 {
		return []chunk{{ChunkID: ragID + "-c0", Text: body, Order: 0}}
	}
	var chunks []chunk
	order := 0
	if locs[0][0] > 0 {
		head := strings.TrimSpace(body[:locs[0][0]])
		if head != "" {
			chunks = append(chunks, chunk{ChunkID: fmt.Sprintf("%s-c%d", ragID, order), Text: head, Order: order})
			order++
		}
	}
	for i, loc := range locs {
		var seg string
		if i+1 < len(locs) {
			seg = body[loc[0]:locs[i+1][0]]
		} else {
			seg = body[loc[0]:]
		}
		if strings.TrimSpace(seg) == "" {
			continue
		}
		chunks = append(chunks, chunk{ChunkID: fmt.Sprintf("%s-c%d", ragID, order), Text: seg, Order: order})
		order++
	}
	return chunks
}

// categoryFromPath suy ra category từ đường dẫn khi không có frontmatter
// và source không khai báo category. Theo taxonomy docs/RAG_STANDARD.md.
func categoryFromPath(path string) string {
	p := strings.ToLower(filepath.ToSlash(path))
	switch {
	case strings.Contains(p, "troubleshooting"):
		return "troubleshooting"
	case strings.Contains(p, "architecture") || strings.Contains(p, "adr"):
		return "architecture"
	case strings.Contains(p, "workflow"):
		return "workflows"
	case strings.Contains(p, "plan"):
		return "plans"
	case strings.Contains(p, "config"):
		return "configuration"
	case strings.Contains(p, "tool") || strings.Contains(p, "reference"):
		return "tools"
	case strings.Contains(p, "integration") || strings.Contains(p, "mcp"):
		return "integration"
	case strings.Contains(p, "test"):
		return "testing"
	default:
		return "guides"
	}
}
