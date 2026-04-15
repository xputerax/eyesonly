package routes

import (
	"net/http"

	"github.com/aimandaniel/eyesonly/db"
	"github.com/go-chi/chi"
)

func SetupRoutes(r *chi.Mux, q *db.Queries) {
	assetsRoute(r)
	SetupSecretsRoute(r, q)
}

func assetsRoute(router *chi.Mux) {
	router.Handle("/static/*",
		http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
}
