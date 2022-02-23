package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/Close-Encounters-Corps/cec-core/pkg/auth/tokens"
	"github.com/Close-Encounters-Corps/cec-core/pkg/config"
	"github.com/Close-Encounters-Corps/cec-core/pkg/discord"
	"github.com/Close-Encounters-Corps/cec-core/pkg/facades"
	"github.com/Close-Encounters-Corps/cec-core/pkg/principal"
	"github.com/Close-Encounters-Corps/cec-core/pkg/users"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v4/pgxpool"
)

type Module interface {
	Start(ctx context.Context) error
}

type Application struct {
	Ctx     context.Context
	Db      *pgxpool.Pool
	Modules map[string]Module
	Config  *config.Config
}

func (app *Application) Start() {
	for k, v := range app.Modules {
		log.Println("Starting module", k)
		err := v.Start(app.Ctx)
		if err != nil {
			log.Fatalln(err)
		}
	}
}

func (app *Application) Handler() {
	assertErr := func(err error, rw http.ResponseWriter) bool {
		if err != nil {
			log.Println(err)
			rw.WriteHeader(http.StatusInternalServerError)
		}
		return err != nil
	}
	dm := app.Modules[discord.MODULE_NAME].(*discord.DiscordModule)
	um := app.Modules[users.MODULE_NAME].(*users.UserModule)
	tm := app.Modules[tokens.MODULE_NAME].(*tokens.TokenModule)
	facade := facades.NewCoreFacade(app.Db, um, dm, tm, app.Config)
	mux := chi.NewMux()
	mux.Use(middleware.Logger)
	// Login using Discord.
	// If state is not provided, then it will return token instead of auth URL
	mux.HandleFunc("/login/discord", func(rw http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		if state == "" {
			u, err := url.Parse(app.Config.AuthExternalUrl)
			if assertErr(err, rw) {
				return
			}
			u, err = u.Parse("/oauth/discord")
			if assertErr(err, rw) {
				return
			}
			rw.WriteHeader(http.StatusOK)
			rw.Write([]byte(u.String()))
			return
		}
		token, err := facade.Authenticate(r.Context(), "discord", state)
		if assertErr(err, rw) {
			return
		}
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte(token))
	})
}

func main() {
	cecdb := requireEnv("CEC_DB")
	authsecret := requireEnv("CEC_AUTH_SECRET")
	authext := requireEnv("CEC_AUTH_EXTERNAL")
	authint := requireEnv("CEC_AUTH_INTERNAL")
	ctx, cancel := context.WithCancel(context.Background())
	db, err := pgxpool.Connect(ctx, cecdb)
	if err != nil {
		log.Fatalln(err)
	}
	app := Application{
		Ctx:     ctx,
		Db:      db,
		Modules: map[string]Module{},
		Config: &config.Config{
			AuthSecret: authsecret,
			AuthInternalUrl: authint,
			AuthExternalUrl: authext,
		},
	}
	pm := principal.NewPrincipalModule()
	app.Modules[principal.MODULE_NAME] = pm
	app.Modules[users.MODULE_NAME] = users.NewUserModule(pm)
	app.Modules[discord.MODULE_NAME] = discord.NewDiscordModule(nil)
	app.Start()
	cancel()
}

func requireEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalln("variable", name, "is unset")
	}
	return value
}
