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
	validation "github.com/go-ozzo/ozzo-validation"
	"github.com/google/uuid"
	"golang.org/x/crypto/pbkdf2"
)

func SetupSecretsRoute(router *chi.Mux, q *db.Queries) {
	router.Get("/", root())
	router.Post("/create", postCreate(q))
	router.Get("/peek/{peekId}", getPeek(router, q))
	router.Post("/peek/{peekId}", postPeek(router, q))
	router.Get("/peek/{peekId}/confirm", confirmPeek())
	router.Get("/edit/{editId}", editForm(router, q))
	router.Post("/edit/{editId}", saveEdit(router, q))
}

func root() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var flashError string
		if cookie, err := r.Cookie("flash_error"); err == nil {
			if decoded, err := base64.URLEncoding.DecodeString(cookie.Value); err == nil {
				flashError = string(decoded)
			}
			// Clear the flash cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "flash_error",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteStrictMode,
			})
		}

		component := views.Home(&views.HomeViewModel{
			FlashError: flashError,
		})

		templ.Handler(component).ServeHTTP(w, r)
	}
}

func postCreate(q *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.Write([]byte(fmt.Sprintf("error parsing form data: %s", err)))
			return
		}

		peekId := uuid.NewString()
		editId := uuid.NewString()

		// TODO: encrypt using given password
		title := r.Form.Get("title")
		content := r.Form.Get("content")
		expiry := r.Form.Get("expiry")
		password := r.Form.Get("password")

		data := struct {
			Title    string
			Content  string
			Expiry   string
			Password string
		}{
			Title:    title,
			Content:  content,
			Expiry:   expiry,
			Password: password,
		}

		validationErrors := validation.ValidateStruct(&data,
			validation.Field(&data.Title,
				validation.Required, validation.NotNil, validation.Length(1, 255), // TODO: max length in schema
			),
			validation.Field(&data.Content,
				validation.Required, validation.NotNil, validation.Length(1, 255), // TODO: max length in schema
			),
			validation.Field(&data.Expiry,
				validation.Date("2006-01-02T15:04"),
			),
			validation.Field(&data.Password),
		)
		if validationErrors != nil {
			http.SetCookie(w, &http.Cookie{
				Name:     "flash_error",
				Value:    base64.URLEncoding.EncodeToString([]byte(validationErrors.Error())),
				Path:     "/",
				MaxAge:   30,
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteStrictMode,
			})
			http.Redirect(w, r, "/", 302)
			return
		}

		var expireAt sql.NullTime
		if parsedTime, err := time.Parse("2006-01-02T15:04", expiry); err != nil {
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
	}
}

func getPeek(router *chi.Mux, q *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
	}
}

func postPeek(router *chi.Mux, q *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
	}
}

func confirmPeek() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
	}
}

func editForm(router *chi.Mux, q *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		editId := chi.URLParam(r, "editId")
		secret, err := q.GetSecretByEditId(r.Context(), editId)
		if err != nil {
			router.NotFoundHandler().ServeHTTP(w, r)
			return
		}

		var flashError string
		if cookie, err := r.Cookie("flash_error"); err == nil {
			if decoded, err := base64.URLEncoding.DecodeString(cookie.Value); err == nil {
				flashError = string(decoded)
			}
			// Clear the flash cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "flash_error",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteStrictMode,
			})
		}

		component := views.Edit(&views.EditViewModel{
			EditId:     editId,
			PeekId:     secret.PeekID,
			PeekURL:    fmt.Sprintf("%s://%s/peek/%s", r.URL.Scheme, r.Host, secret.PeekID),
			Title:      secret.Title,
			Content:    secret.Content,
			FlashError: flashError,
		})

		templ.Handler(component).ServeHTTP(w, r)
	}
}

func saveEdit(router *chi.Mux, q *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		newTitle := r.Form.Get("title")
		newContent := r.Form.Get("content")

		data := struct {
			Title   string
			Content string
		}{
			Title:   newTitle,
			Content: newContent,
		}

		validationErrors := validation.ValidateStruct(&data,
			validation.Field(&data.Title,
				validation.NotNil, validation.Required, validation.Length(1, 255)),
			validation.Field(&data.Content,
				validation.NotNil, validation.Required, validation.Length(1, 255)), // TODO: should probably increase the length in schema
		)
		if validationErrors != nil {
			http.SetCookie(w, &http.Cookie{
				Name:     "flash_error",
				Value:    base64.URLEncoding.EncodeToString([]byte(validationErrors.Error())),
				Path:     "/",
				MaxAge:   30,
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteStrictMode,
			})
			http.Redirect(w, r, "/edit/"+editId, 302)
			return
		}

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
	}
}
