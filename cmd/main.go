package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/Close-Encounters-Corps/cec-core/pkg/auth/tokens"
	"github.com/Close-Encounters-Corps/cec-core/pkg/config"
	"github.com/Close-Encounters-Corps/cec-core/pkg/controllers"
	"github.com/Close-Encounters-Corps/cec-core/pkg/discord"
	"github.com/Close-Encounters-Corps/cec-core/pkg/facades"
	"github.com/Close-Encounters-Corps/cec-core/pkg/principal"
	"github.com/Close-Encounters-Corps/cec-core/pkg/tracer"
	"github.com/Close-Encounters-Corps/cec-core/pkg/users"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
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



func (app *Application) Server() (*gin.Engine, error) {
	dm := app.Modules[discord.MODULE_NAME].(*discord.DiscordModule)
	um := app.Modules[users.MODULE_NAME].(*users.UserModule)
	tm := app.Modules[tokens.MODULE_NAME].(*tokens.TokenModule)
	facade := facades.NewCoreFacade(app.Db, um, dm, tm, app.Config)
	ctrl := controllers.CoreController{
		Facade: facade,
		Config: app.Config,
	}
	r := gin.Default()
	v1 := r.Group("/v1")
	v1.Use(otelgin.Middleware("v1"))
	v1.GET("/login/discord", ctrl.LoginDiscord)
	v1.GET("/users/current", ctrl.CurrentUser)
	return r, nil
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
