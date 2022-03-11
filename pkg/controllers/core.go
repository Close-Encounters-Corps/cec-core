package controllers

import (
	"context"
	"log"
	"net/http"
	"net/url"

	"github.com/Close-Encounters-Corps/cec-core/pkg/api/httpapi"
	"github.com/Close-Encounters-Corps/cec-core/pkg/config"
	"github.com/Close-Encounters-Corps/cec-core/pkg/facades"
	"github.com/Close-Encounters-Corps/cec-core/pkg/tracer"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

type CoreController struct {
	Facade *facades.CoreFacade
	Config *config.Config
}

type RequestHelper struct {
	Req     *gin.Context
	Ctx     context.Context
	Span    trace.Span
	TraceID string
}

func NewRequestHelper(c *gin.Context, path string) *RequestHelper {
	ctx, span := tracer.NewSpan(c.Request.Context(), path, nil)
	return &RequestHelper{
		Req:     c,
		Ctx:     ctx,
		Span:    span,
		TraceID: span.SpanContext().TraceID().String(),
	}
}

func (help *RequestHelper) InternalError(err error) {
	tracer.AddSpanError(help.Span, err)
	tracer.FailSpan(help.Span, "internal error")
	log.Printf("[%s] error: %s\n", help.TraceID, err)
	help.Req.JSON(http.StatusOK, httpapi.Error{
		RequestID: help.TraceID,
	})
}


func (ctrl *CoreController) LoginDiscord(c *gin.Context) {
	help := NewRequestHelper(c, "/login/discord")
	defer help.Span.End()
	internalError := func(err error) {
		help.InternalError(err)
	}
	state := c.Query("state")
	if state == "" {
		// respond with cec-auth url as nextURL
		u, err := url.Parse(ctrl.Config.AuthExternalUrl)
		if err != nil {
			internalError(err)
			return
		}
		u, err = u.Parse("/oauth/discord")
		if err != nil {
			internalError(err)
			return
		}
		q := u.Query()
		q.Add("redirect_url", c.Query("success_url"))
		u.RawQuery = q.Encode()
		result := httpapi.AuthPhaseResult{
			Phase:   1,
			NextURL: u.String(),
		}
		c.JSON(http.StatusOK, &result)
		return
	}
	tracer.AddSpanTags(help.Span, map[string]string{"state": state})
	token, err := ctrl.Facade.Authenticate(help.Ctx, "discord", state)
	if err != nil {
		if err.Error() == "state not found" {
			c.JSON(http.StatusBadRequest, httpapi.Error{
				Message:   "state not found",
				RequestID: help.TraceID,
			})
			return
		}
		internalError(err)
		return
	}
	result := httpapi.AuthPhaseResult{
		Phase: 2,
		Token: token,
	}
	c.JSON(http.StatusOK, result)
}


func (ctrl *CoreController) CurrentUser(c *gin.Context) {
	help := NewRequestHelper(c, "controller.users.current")
	defer help.Span.End()
	token := c.Request.Header.Get("X-Auth-Token")
	if token == "" {
		c.JSON(http.StatusBadRequest, httpapi.Error{
			Message:   "token not provided",
			RequestID: help.TraceID,
		})
		return
	}
	user, err := ctrl.Facade.CurrentUser(help.Ctx, token)
	if err != nil {
		help.InternalError(err)
		return
	}
	c.JSON(http.StatusOK, user)
}

func (ctrl *CoreController) PromoteToAdmin(c *gin.Context) {
	help := NewRequestHelper(c, "controller.user.promote")
	defer help.Span.End()
	token := c.GetHeader("X-Auth-Token")
	if token == "" {
		c.JSON(http.StatusBadRequest, httpapi.Error{
			Message: "token not provided",
			RequestID: help.TraceID,
		})
		return
	}
	err := ctrl.Facade.PromoteToAdmin(help.Ctx, token)
	if err != nil {
		help.InternalError(err)
		return
	}
	c.Status(http.StatusOK)
}