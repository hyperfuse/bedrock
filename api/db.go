package api

// "github.com/go-chi/jwtauth/v5"
// "github.com/go-chi/jwtauth"

// "hyperfuse.co/ratio/sqlc/db"

type contextKey string

const (
	databaseKey contextKey = "database"
	userKey     contextKey = "user"
	emailKey    contextKey = "email"
)

// func DBContext(pool *pgxpool.Pool) func(next http.Handler) http.Handler {
// 	return func(next http.Handler) http.Handler {
// 		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			conn, err := pool.Acquire(r.Context())
// 			log.Debug().Msg("Acquired connection")
// 			if err != nil {
// 				write(w, http.StatusInternalServerError, Error{Message: err.Error()})
// 				return
// 			}
// 			q := db.New(conn)
// 			ctx := context.WithValue(r.Context(), databaseKey, q)
// 			next.ServeHTTP(w, r.WithContext(ctx))
// 			log.Debug().Msg("Releasing connection")
// 			conn.Release()
// 		})
// 	}
// }

// func UserContext(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		ctx := r.Context()
// 		_, claim, err := jwtauth.FromContext(r.Context())
// 		if err != nil {
// 			write(w, http.StatusInternalServerError, Error{Message: err.Error()})
// 			return
// 		}
// 		email := ""
// 		if claim[string(emailKey)] != nil {
// 			email = claim[string(emailKey)].(string)
// 		}

// 		if email == "" {
// 			write(w, http.StatusInternalServerError, Error{Message: "No email in token"})
// 			return
// 		}

// 		q := ctx.Value(databaseKey).(*db.Queries)

// 		user, err := q.GetUserByEmail(ctx, email)
// 		if err != nil {
// 			if err == pgx.ErrNoRows {
// 				write(w, http.StatusInternalServerError, Error{Message: err.Error()})
// 				return
// 			} else {
// 				write(w, http.StatusInternalServerError, Error{Message: err.Error()})
// 				return
// 			}
// 		}
// 		ctx = context.WithValue(ctx, userKey, user)
// 		next.ServeHTTP(w, r.WithContext(ctx))
// 	})
// }
