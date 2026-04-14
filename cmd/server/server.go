package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/a-h/templ"
	"github.com/aimandaniel/eyesonly/db"
	"github.com/aimandaniel/eyesonly/routes"
	"github.com/aimandaniel/eyesonly/views"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	_ "github.com/mattn/go-sqlite3"
)

type M map[string]interface{}

func main() {
	sqliteConn, err := sql.Open("sqlite3", "eyesonly.sqlite3")
	if err != nil {
		slog.Error(fmt.Sprintf("failed to open database connection: %s", err))
		os.Exit(-1)
	}

	q := db.New(sqliteConn)
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found :("))
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("method not allowed bruh, what are u trying to do?"))
	})

	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		component := views.Home()
		templ.Handler(component).ServeHTTP(w, r)
	})

	routes.SetupRoutes(router, q)

	slog.Info("starting server at :6969")
	if err := http.ListenAndServe("0.0.0.0:6969", router); err != nil {
		slog.Error(fmt.Sprintf("failed to start server: %s", err))
	}
}
