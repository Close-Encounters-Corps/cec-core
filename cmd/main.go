package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/Close-Encounters-Corps/cec-core/gen/models"
	"github.com/Close-Encounters-Corps/cec-core/gen/restapi"
	"github.com/Close-Encounters-Corps/cec-core/gen/restapi/operations"
	"github.com/Close-Encounters-Corps/cec-core/gen/restapi/operations/auth"
	apiusers "github.com/Close-Encounters-Corps/cec-core/gen/restapi/operations/users"
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
	"go.opentelemetry.io/otel/trace"
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

type RequestHelper struct {
	Req     *http.Request
	Ctx     context.Context
	Span    trace.Span
	TraceID string
}

func NewRequestHelper(req *http.Request, path string) *RequestHelper {
	ctx, span := tracer.NewSpan(req.Context(), path, nil)
	return &RequestHelper{
		Req:     req,
		Ctx:     ctx,
		Span:    span,
		TraceID: span.SpanContext().TraceID().String(),
	}
}

func (help *RequestHelper) InternalError(err error) {
	tracer.AddSpanError(help.Span, err)
	tracer.FailSpan(help.Span, "internal error")
	log.Printf("[%s] error: %s", help.TraceID, err)
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
			help := NewRequestHelper(ldp.HTTPRequest, "/login/discord")
			defer help.Span.End()
			internalError := func(err error) middleware.Responder {
				help.InternalError(err)
				return auth.NewLoginDiscordInternalServerError().WithPayload(&models.Error{
					RequestID: help.TraceID,
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
			tracer.AddSpanTags(help.Span, map[string]string{"state": state})
			token, err := facade.Authenticate(help.Ctx, "discord", state)
			if err != nil {
				if err.Error() == "state not found" {
					return auth.NewLoginDiscordBadRequest().WithPayload(&models.Error{
						Message:   "state not found",
						RequestID: help.TraceID,
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
	api.UsersGetUsersCurrentHandler = apiusers.GetUsersCurrentHandlerFunc(
		func(gucp apiusers.GetUsersCurrentParams) middleware.Responder {
			help := NewRequestHelper(gucp.HTTPRequest, "/users/current")
			defer help.Span.End()
			token := swag.StringValue(gucp.XAuthToken)
			if token == "" {
				return apiusers.NewGetUsersCurrentBadRequest().WithPayload(
					&models.Error{
						Message:   "token not provided",
						RequestID: help.TraceID,
					},
				)
			}
			user, err := facade.CurrentUser(help.Ctx, token)
			if err != nil {
				help.InternalError(err)
				return apiusers.NewGetUsersCurrentInternalServerError().WithPayload(
					&models.Error{RequestID: help.TraceID},
				)
			}
			return apiusers.NewGetUsersCurrentOK().WithPayload(&models.User{
				ID: int64(user.Id),
				Principal: &models.Principal{
					ID:        int64(user.Principal.Id),
					Admin:     user.Principal.Admin,
					CreatedOn: user.Principal.CreatedOn.Format(time.RFC3339),
					LastLogin: user.Principal.LastLogin.Format(time.RFC3339),
					State:     user.Principal.State,
				},
			})
		},
	)
	server := restapi.NewServer(api)
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
