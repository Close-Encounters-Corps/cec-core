package main

import (
	"context"
	"log"
	"net/url"
	"os"
	"strconv"

	"github.com/Close-Encounters-Corps/cec-core/gen/models"
	"github.com/Close-Encounters-Corps/cec-core/gen/restapi"
	"github.com/Close-Encounters-Corps/cec-core/gen/restapi/operations"
	"github.com/Close-Encounters-Corps/cec-core/gen/restapi/operations/auth"
	"github.com/Close-Encounters-Corps/cec-core/pkg/auth/tokens"
	"github.com/Close-Encounters-Corps/cec-core/pkg/config"
	"github.com/Close-Encounters-Corps/cec-core/pkg/discord"
	"github.com/Close-Encounters-Corps/cec-core/pkg/facades"
	"github.com/Close-Encounters-Corps/cec-core/pkg/principal"
	"github.com/Close-Encounters-Corps/cec-core/pkg/users"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
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

var COMMITSHA string

func (app *Application) Start() {
	for k, v := range app.Modules {
		log.Println("Starting module", k)
		err := v.Start(app.Ctx)
		if err != nil {
			log.Fatalln(err)
		}
	}
}

func (app *Application) Server() (*restapi.Server, error) {
	swagger, err := loads.Analyzed(restapi.SwaggerJSON, "")
	if err != nil {
		return nil, err
	}
	dm := app.Modules[discord.MODULE_NAME].(*discord.DiscordModule)
	um := app.Modules[users.MODULE_NAME].(*users.UserModule)
	tm := app.Modules[tokens.MODULE_NAME].(*tokens.TokenModule)
	facade := facades.NewCoreFacade(app.Db, um, dm, tm, app.Config)
	api := operations.NewCecCoreAPI(swagger)
	api.AuthLoginDiscordHandler = auth.LoginDiscordHandlerFunc(
		func(ldp auth.LoginDiscordParams) middleware.Responder {
			state := swag.StringValue(ldp.State)
			if state == "" {
				// respond with cec-auth url as nextURL
				u, err := url.Parse(app.Config.AuthExternalUrl)
				if err != nil {
					log.Println(err)
					return auth.NewLoginDiscordInternalServerError()
				}
				u, err = u.Parse("/oauth/discord")
				if err != nil {
					log.Println(err)
					return auth.NewLoginDiscordInternalServerError()
				}
				u.Query().Add("redirect_url", swag.StringValue(ldp.SuccessURL))
				result := models.AuthPhaseResult{
					Phase:   1,
					NextURL: u.String(),
				}
				return auth.NewLoginDiscordOK().WithPayload(&result)
			}
			token, err := facade.Authenticate(ldp.HTTPRequest.Context(), "discord", state)
			if err != nil {
				log.Println(err)
				return auth.NewLoginDiscordInternalServerError()
			}
			result := models.AuthPhaseResult{
				Phase: 2,
				Token: token,
			}
			return auth.NewLoginDiscordOK().WithPayload(&result)
		},
	)
	server := restapi.NewServer(api)
	server.Port = app.Config.Listen
	return server, nil
}

func main() {
	cecdb := requireEnv("CEC_DB")
	authsecret := requireEnv("CEC_AUTH_SECRET")
	authext := requireEnv("CEC_AUTH_EXTERNAL")
	authint := requireEnv("CEC_AUTH_INTERNAL")
	listenport := requireEnv("CEC_LISTENPORT")
	ctx, cancel := context.WithCancel(context.Background())
	db, err := pgxpool.Connect(ctx, cecdb)
	if err != nil {
		log.Fatalln(err)
	}
	port, err := strconv.Atoi(listenport)
	if err != nil {
		log.Fatalln(err)
	}
	app := Application{
		Ctx:     ctx,
		Db:      db,
		Modules: map[string]Module{},
		Config: &config.Config{
			Listen:          port,
			AuthSecret:      authsecret,
			AuthInternalUrl: authint,
			AuthExternalUrl: authext,
		},
	}
	pm := principal.NewPrincipalModule()
	app.Modules[principal.MODULE_NAME] = pm
	app.Modules[users.MODULE_NAME] = users.NewUserModule(pm)
	app.Modules[discord.MODULE_NAME] = discord.NewDiscordModule(nil)
	app.Start()
	server, err := app.Server()
	if err != nil {
		log.Fatalln(err)
	}
	defer server.Shutdown()
	log.Println("Commit:", COMMITSHA)
	log.Println("Ready.")
	if err := server.Serve(); err != nil {
		cancel()
		log.Fatalln(err)
	}
}

func requireEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalln("variable", name, "is unset")
	}
	return value
}
