package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 创建一个简单的 zip 用于测试
func makeTestZip(files map[string]string) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, content := range files {
		f, _ := zw.Create(name)
		f.Write([]byte(content))
	}
	zw.Close()
	return buf.Bytes()
}

func postAppidImportZip(s *Server, token string, zipData []byte) (int, string) {
	req := httptest.NewRequest("POST", "/api/v1/appid/import", bytes.NewReader(zipData))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/zip")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	return rr.Code, rr.Body.String()
}

func TestAppidImportZipPathTraversal(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	tok := loginTokenAs(t, s, "admin", "admin123")

	// 恶意 entry：路径穿越
	evil := makeTestZip(map[string]string{
		"../../etc/passwd": "root:x:0:0:root:/root:/bin/bash",
		"../evil":          "bad",
		"sub/../../escape": "oops",
	})
	code, body := postAppidImportZip(s, tok, evil)
	t.Logf("evil zip response: code=%d body=%s", code, body)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("路径穿越应 422，得 %d: %s", code, body)
	}
	var resp map[string]interface{}
	_ = json.NewDecoder(strings.NewReader(body)).Decode(&resp)
	t.Logf("parsed resp: %+v, code field type=%T value=%v", resp, resp["code"], resp["code"])
	// code 可能是 string 或 float64，统一比较
	codeVal := fmt.Sprintf("%v", resp["code"])
	if codeVal != "11908" { // unsafe entry name
		t.Fatalf("应返回 11908 unsafe entry name，实际 code=%v", resp["code"])
	}

	// 正常 zip：不应返回 11908（unsafe entry name），权限错误 11906 可接受（测试环境非 root）
	good := makeTestZip(map[string]string{
		"bilibili": "*.bilibili.com\n",
		"douyin":   "*.douyin.com\n",
	})
	code, body = postAppidImportZip(s, tok, good)
	if code == http.StatusUnprocessableEntity {
		var r map[string]interface{}
		_ = json.NewDecoder(strings.NewReader(body)).Decode(&r)
		codeVal := fmt.Sprintf("%v", r["code"])
		if codeVal == "11908" {
			t.Fatalf("正常 zip 不应触发 11908: %s", body)
		}
	}
}

func TestAppidImportZipLimits(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	tok := loginTokenAs(t, s, "admin", "admin123")

	// 超过条目数上限
	many := make(map[string]string)
	for i := 0; i < 2005; i++ {
		many[fmt.Sprintf("app%d", i)] = "test.com\n"
	}
	code, body := postAppidImportZip(s, tok, makeTestZip(many))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("条目数超限应 422，得 %d: %s", code, body)
	}
}

func TestAppidImportMultipart(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	tok := loginTokenAs(t, s, "admin", "admin123")

	// 正常 multipart
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("bilibili", "bilibili")
	fw.Write([]byte("*.bilibili.com\n"))
	fw2, _ := w.CreateFormFile("douyin", "douyin")
	fw2.Write([]byte("*.douyin.com\n"))
	w.Close()

	req := httptest.NewRequest("POST", "/api/v1/appid/import", &buf)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	// 测试环境非 root 会报 11906 权限错误，只要不是 11908/11903 即通过
	if rr.Code == http.StatusUnprocessableEntity {
		var r map[string]interface{}
		_ = json.NewDecoder(rr.Body).Decode(&r)
		codeVal := fmt.Sprintf("%v", r["code"])
		if codeVal == "11908" || codeVal == "11903" {
			t.Fatalf("multipart 正常不应触发 11908/11903: %s", rr.Body.String())
		}
	}

	// multipart 带路径穿越文件名
	buf.Reset()
	w = multipart.NewWriter(&buf)
	fw, _ = w.CreateFormFile("bilibili", "../../etc/passwd")
	fw.Write([]byte("bad"))
	w.Close()

	req = httptest.NewRequest("POST", "/api/v1/appid/import", &buf)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr = httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	t.Logf("multipart traversal response: code=%d body=%s", rr.Code, rr.Body.String())
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("multipart 路径穿越应 422，得 %d: %s", rr.Code, rr.Body.String())
	}
	var r map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&r)
	codeVal := fmt.Sprintf("%v", r["code"])
	if codeVal != "11908" {
		t.Fatalf("multipart 路径穿越应返回 11908，实际 code=%v", r["code"])
	}
}

func TestAppidImportJSONKeySafety(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	tok := loginTokenAs(t, s, "admin", "admin123")

	// JSON key 含路径穿越
	body := `{"files":{"../../evil":"bad.com\n"},"version":"1"}`
	req := httptest.NewRequest("POST", "/api/v1/appid/import", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("JSON key 路径穿越应 422，得 %d: %s", rr.Code, rr.Body.String())
	}
}
