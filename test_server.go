package main

import (
    "net/http"
    "github.com/go-chi/chi/v5"
)

func main() {
    mux := chi.NewRouter()
    fs := http.FileServer(http.Dir("web/gantt"))
    mux.Handle("/gantt/*", http.StripPrefix("/gantt/", fs))
    go http.ListenAndServe(":8081", mux)
}
