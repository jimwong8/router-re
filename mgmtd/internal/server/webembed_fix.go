package server
import ("net/http"; "strings")
func handleWebEmbed() http.HandlerFunc {
 return func(w http.ResponseWriter, r *http.Request) {
  if strings.HasPrefix(r.URL.Path,"/api/"){w.WriteHeader(404);return}
  w.Header().Set("Cache-Control","no-cache")
  w.Write([]byte("<html><body>embed ok</body></html>"))
 }}
