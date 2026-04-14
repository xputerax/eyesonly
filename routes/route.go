package routes

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/aimandaniel/eyesonly/db"
	"github.com/aimandaniel/eyesonly/utils"
	"github.com/aimandaniel/eyesonly/views"
	"github.com/go-chi/chi"
	"github.com/google/uuid"
	"golang.org/x/crypto/pbkdf2"
)

func SetupRoutes(r *chi.Mux, q *db.Queries) {
	secretsRoute(r, q)
}

func secretsRoute(router *chi.Mux, q *db.Queries) {
	router.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.Write([]byte(fmt.Sprintf("error parsing form data: %s", err)))
			return
		}

		peekId := uuid.NewString()
		editId := uuid.NewString()

		// TODO: form validation
		// TODO: encrypt using given password
		title := r.Form.Get("title")
		content := r.Form.Get("content")
		expiry := r.Form.Get("expiry")
		password := r.Form.Get("password")

		var expireAt sql.NullTime
		if parsedTime, err := time.Parse("2001-02-03", expiry); err != nil {
			expireAt.Time = parsedTime
		}

		var passwordHash sql.NullString
		if password != "" {
			passwordHash.Valid = true
			passwordHash.String = password

			round := 10000

			salt := make([]byte, 16)
			if _, err := rand.Read(salt); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(fmt.Sprintf("failed to generate random salt: %s", err)))
				return
			}

			slog.Info("generated salt", "salt", salt, "len", len(salt))

			key := pbkdf2.Key([]byte(password), salt, round, 32, sha512.New)

			slog.Info("generated key", "key", key, "len", len(key))

			saltStr := base64.StdEncoding.EncodeToString(salt)

			slog.Info("converted salt to base64", "content", saltStr, "len", len(saltStr))

			contentPrefix := fmt.Sprintf("%s:%d", saltStr, round)

			slog.Info("this will be prepended to the content", "prefix", contentPrefix)

			b, err := aes.NewCipher(key)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(fmt.Sprintf("failed to create cipher: %s", err)))
				return
			}

			iv := make([]byte, 16)
			bm := cipher.NewCBCEncrypter(b, iv)
			paddedText := utils.PadPKCS7([]byte(content))

			var encrypted []byte = make([]byte, len(paddedText))
			bm.CryptBlocks(encrypted, paddedText)

			slog.Info("performing AES", "input", content, "output", encrypted)

			d := cipher.NewCBCDecrypter(b, iv)
			var decrypted []byte = make([]byte, len(paddedText))
			d.CryptBlocks(decrypted, encrypted)

			unpadded, err := utils.UnpadPKCS7(decrypted)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(fmt.Sprintf("error when unpadding decrypted text: %s", err)))
				return
			}

			slog.Info("decrypted aes", "text", string(decrypted), "unpadded", string(unpadded))

		} else {
			// TODO: error
		}

		row, err := q.InsertSecret(r.Context(), db.InsertSecretParams{
			EditID:   editId,
			PeekID:   peekId,
			Title:    title,
			Content:  content,
			ExpireAt: expireAt,
			Password: passwordHash,
		})

		if err != nil {
			w.Write([]byte(fmt.Sprintf("error when inserting secret: %s", err)))
			return
		}

		slog.Info("created secret message", "secret", map[string]any{
			"title":    row.Title,
			"content":  row.Content,
			"expiry":   row.ExpireAt,
			"password": row.Password,
		})

		http.Redirect(w, r, "/edit/"+editId, 303)
	})

	router.Get("/peek/{peekId}", func(w http.ResponseWriter, r *http.Request) {
		peekId := chi.URLParam(r, "peekId")

		slog.Info("peeking message",
			"peekId", peekId)

		secret, err := q.GetSecretByPeekId(r.Context(), peekId)
		if err != nil {
			router.NotFoundHandler().ServeHTTP(w, r)
			return
		}

		slog.Info(fmt.Sprintf("found secret = %+v", secret))

		component := views.PeekConfirmation(&views.PeekConfirmationViewModel{
			PeekId: peekId,
		})

		templ.Handler(component).ServeHTTP(w, r)
	})

	router.Post("/peek/{peekId}", func(w http.ResponseWriter, r *http.Request) {
		peekId := chi.URLParam(r, "peekId")

		slog.Info("confirm peek message",
			"peekId", peekId)

		secret, err := q.GetSecretByPeekId(r.Context(), peekId)
		if err != nil {
			router.NotFoundHandler().ServeHTTP(w, r)
			return
		}

		// TODO: check expiry
		// TODO: check password

		if _, err := q.DeleteSecretByPeekId(r.Context(), peekId); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("failed to delete secret: %s", err)))
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "flash_title",
			Value:    base64.URLEncoding.EncodeToString([]byte(secret.Title)),
			Path:     "/",
			MaxAge:   30,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})
		http.SetCookie(w, &http.Cookie{
			Name:     "flash_content",
			Value:    base64.URLEncoding.EncodeToString([]byte(secret.Content)),
			Path:     "/",
			MaxAge:   30,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})

		// POST-Redirect-GET (PRG) pattern
		http.Redirect(w, r, "/peek/"+peekId+"/confirm", http.StatusSeeOther)
	})

	router.Get("/peek/{peekId}/confirm", func(w http.ResponseWriter, r *http.Request) {
		titleCookie, err := r.Cookie("flash_title")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(fmt.Sprintf("flash_title cookie not found")))
			return
		}

		contentCookie, err := r.Cookie("flash_content")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(fmt.Sprintf("flash_content cookie not found")))
			return
		}

		titleBytes, err := base64.URLEncoding.DecodeString(titleCookie.Value)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("failed to decode flash_title cookie")))
			return
		}

		contentBytes, err := base64.URLEncoding.DecodeString(contentCookie.Value)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("failed to decode flash_content cookie")))
			return
		}

		title := string(titleBytes)
		content := string(contentBytes)

		slog.Info("decoding secrets", "titleBytes", titleBytes, "title", title, "contentBytes", contentBytes, "content", content)

		component := views.Peek(&views.PeekViewModel{
			Title:   title,
			Content: content,
		})

		templ.Handler(component).ServeHTTP(w, r)
	})

	router.Get("/edit/{editId}", func(w http.ResponseWriter, r *http.Request) {
		editId := chi.URLParam(r, "editId")
		secret, err := q.GetSecretByEditId(r.Context(), editId)
		if err != nil {
			router.NotFoundHandler().ServeHTTP(w, r)
			return
		}

		component := views.Edit(&views.EditViewModel{
			EditId:  editId,
			PeekId:  secret.PeekID,
			PeekURL: fmt.Sprintf("%s://%s/peek/%s", r.URL.Scheme, r.Host, secret.PeekID),
			Title:   secret.Title,
			Content: secret.Content,
		})

		templ.Handler(component).ServeHTTP(w, r)
	})

	router.Post("/edit/{editId}", func(w http.ResponseWriter, r *http.Request) {
		editId := chi.URLParam(r, "editId")
		_, err := q.GetSecretByEditId(r.Context(), editId)
		if err != nil {
			router.NotFoundHandler().ServeHTTP(w, r)
			return
		}

		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(fmt.Sprintf("failed to parse form body: %s", err)))
			return
		}

		// TODO: form validation
		newTitle := r.Form.Get("title")
		newContent := r.Form.Get("content")

		slog.Info("editing secret",
			"editId", editId,
			"newTitle", newTitle,
			"newContent", newContent)

		_, err = q.UpdateSecretByEditId(r.Context(), db.UpdateSecretByEditIdParams{
			Title:   newTitle,
			Content: newContent,
			EditID:  editId,
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("failed to edit secret: %s", err)))
			return
		}

		http.RedirectHandler(fmt.Sprintf("/edit/%s", editId), http.StatusFound).ServeHTTP(w, r)
	})
}
