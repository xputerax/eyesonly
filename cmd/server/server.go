package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/a-h/templ"
	"github.com/aimandaniel/eyesonly/db"
	"github.com/aimandaniel/eyesonly/views"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/pbkdf2"
)

const padByte byte = 0x01

type M map[string]interface{}

func unpadPKCS7(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 || ciphertext[len(ciphertext)-1] != padByte {
		return nil, errors.New("invalid PKCS7 padding")
	}
	unpaddedText := make([]byte, len(ciphertext)-1)
	copy(unpaddedText, ciphertext[:len(ciphertext)-1])
	return unpaddedText, nil
}

// Pads the given plaintext data using PKCS7 padding,
// and returns the padded text as a byte slice.
func padPKCS7(plaintext []byte) []byte {
	if len(plaintext)%aes.BlockSize == 0 {
		return plaintext
	}
	padding := aes.BlockSize - (len(plaintext) % aes.BlockSize)
	paddedText := make([]byte, len(plaintext)+padding)
	copy(paddedText, plaintext)
	for i := len(plaintext); i < len(paddedText); i++ {
		paddedText[i] = padByte
	}
	return paddedText
}

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
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(fmt.Sprintf("error generating key: %s", err)))
				return
			}

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
			paddedText := padPKCS7([]byte(content))

			var encrypted []byte = make([]byte, len(paddedText))
			bm.CryptBlocks(encrypted, paddedText)

			slog.Info("performing AES", "input", content, "output", encrypted)

			d := cipher.NewCBCDecrypter(b, iv)
			var decrypted []byte = make([]byte, len(paddedText))
			d.CryptBlocks(decrypted, encrypted)

			unpadded, err := unpadPKCS7(decrypted)
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

		w.Write([]byte("creating secret message"))

		slog.Info("created secret message", "secret", M{
			"title":    row.Title,
			"content":  row.Content,
			"expiry":   row.ExpireAt,
			"password": row.Password,
		})
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

		component := views.PeekConfirmation(views.PeekConfirmationViewModel{
			PeekId: peekId,
		})

		templ.Handler(component).ServeHTTP(w, r)
	})

	router.Post("/peek/{peekId}/confirm", func(w http.ResponseWriter, r *http.Request) {
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

		component := views.Peek(views.PeekViewModel{
			Title:   secret.Title,
			Content: secret.Content,
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

	slog.Info("starting server at :6969")
	if err := http.ListenAndServe("0.0.0.0:6969", router); err != nil {
		slog.Error("failed to start server: %s", err)
	}
}
