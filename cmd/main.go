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
	"github.com/Close-Encounters-Corps/cec-core/pkg/tracer"
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
	Tracer  *tracer.Tracer
}

var COMMITSHA string
var APPLICATION_NAME = "cec-core"
var VERSION = "0.1.0"

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
			ctx, span := tracer.NewSpan(ldp.HTTPRequest.Context(), "/login/discord", nil)
			defer span.End()
			traceId := span.SpanContext().TraceID().String()
			internalError := func(err error) middleware.Responder {
				tracer.AddSpanError(span, err)
				tracer.FailSpan(span, "internal error")
				log.Printf("[%s] error: %s", traceId, err)
				return auth.NewLoginDiscordInternalServerError().WithPayload(&models.Error{
					RequestID: traceId,
				})
			}
			state := swag.StringValue(ldp.State)
			if state == "" {
				// respond with cec-auth url as nextURL
				u, err := url.Parse(app.Config.AuthExternalUrl)
				if err != nil {
					return internalError(err)
				}
				u, err = u.Parse("/oauth/discord")
				if err != nil {
					return internalError(err)
				}
				q := u.Query()
				q.Add("redirect_url", swag.StringValue(ldp.SuccessURL))
				u.RawQuery = q.Encode()
				result := models.AuthPhaseResult{
					Phase:   1,
					NextURL: u.String(),
				}
				return auth.NewLoginDiscordOK().WithPayload(&result)
			}
			tracer.AddSpanTags(span, map[string]string{"state": state})
			token, err := facade.Authenticate(ctx, "discord", state)
			if err != nil {
				if err.Error() == "state not found" {
					return auth.NewLoginDiscordBadRequest().WithPayload(&models.Error{
						Message: "state not found",
						RequestID: traceId,
					})
				}
				return internalError(err)
			}
			result := models.AuthPhaseResult{
				Phase: 2,
				Token: token,
			}
			return auth.NewLoginDiscordOK().WithPayload(&result)
		},
	)
	server := restapi.NewServer(api)
	server.ConfigureAPI()
	server.SetHandler(tracer.HTTPHandler(server.GetHandler(), "/v1"))
	server.Port = app.Config.Listen
	return server, nil
}

func main() {
	env := requireEnv("CEC_ENVIRONMENT")
	cecdb := requireEnv("CEC_DB")
	jaeger := requireEnv("CEC_JAEGER")
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
	app.Tracer, err = tracer.SetupTracing(&tracer.TracerConfig{
		ServiceName: APPLICATION_NAME,
		ServiceVer:  VERSION,
		Jaeger:      jaeger,
		Environment: env,
		Disabled:    false,
	})
	if err != nil {
		log.Fatalln(err)
	}
	pm := principal.NewPrincipalModule()
	app.Modules[principal.MODULE_NAME] = pm
	app.Modules[users.MODULE_NAME] = users.NewUserModule(pm)
	app.Modules[discord.MODULE_NAME] = discord.NewDiscordModule(nil)
	app.Modules[tokens.MODULE_NAME] = tokens.NewTokenModule()
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
