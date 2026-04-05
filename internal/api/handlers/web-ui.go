package handlers

import (
	"net/http"
    "html/template"

)

// WebUIHandler отдаёт HTML-страницу веб-интерфейса.
type WebUIHandler struct{}
//
// func (h *WebUIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
// 	http.ServeFile(w, r, filepath.Join("web", "static", "index.html"))
// }

type PageData struct {
	Title   string
	Content string
}

func (h *WebUIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse the template file
	// Note: In production, templates are often parsed once at startup for efficiency
	t, err := template.ParseFiles("web/static/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create a data instance
	data := PageData{
		Title:   "My Awesome Website",
		Content: "Welcome to the homepage!",
	}

	// Execute the template, writing the output to the http.ResponseWriter
	err = t.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}