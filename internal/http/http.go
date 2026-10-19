package http

import (
	"embed"
	"errors"
	"fmt"
	"github.com/avanha/pmaas-plugin-fakes/data"
	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	"github.com/avanha/pmaas-spi"
	"github.com/avanha/pmaas-spi/format"
	"github.com/avanha/pmaas-spi/logging"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"reflect"
)

//go:embed content/static content/templates
var contentFS embed.FS

var devicesTemplate = spi.TemplateInfo{
	Name: "fakes_devices",
	FuncMap: template.FuncMap{
		"RelativeTime": format.Ago,
		"Temperature":  format.Temperature,
	},
	Paths:   []string{"templates/fakes_devices.htmlt"},
	Styles:  []string{"css/fakes_devices.css"},
	Scripts: []string{"js/fakes_devices.js"},
}

// Handler owns the plugin's HTTP surface: the page that shows the fakes, and the route that changes them.
type Handler struct {
	log       *slog.Logger
	container spi.IPMAASContainer
	store     common.FakeStore
}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) Init(container spi.IPMAASContainer, store common.FakeStore) {
	h.container = container
	h.log = logging.For(container, "http")
	h.store = store

	container.ProvideContentFS(&contentFS, "content")
	container.EnableStaticContent("static")
	container.AddRouteWithOptions(
		"",
		h.handleHttpListRequest,
		&spi.HttpHandlerOptions{SupportsXsrfValidation: true, MenuLabel: "Fakes"})
	container.AddJsonRoute(
		"control",
		func() any { return &controlRequest{} },
		h.handleHttpControlRequest)
	container.RegisterEntityRenderer(
		reflect.TypeOf((*data.Fakes)(nil)).Elem(),
		h.devicesRendererFactory)
}

func (h *Handler) handleHttpListRequest(w http.ResponseWriter, r *http.Request) {
	fakes, err := h.store.GetFakes()

	if err != nil {
		h.log.Error("unable to retrieve the fakes", "error", err)
		fakes = data.Fakes{}
	}

	h.container.RenderList(w, r, spi.RenderListOptions{Title: "Fakes"}, []any{&fakes})
}

type controlRequest struct {
	Name   string
	Action string

	// Count is for the actions that take a whole number, and Value, in degrees Fahrenheit, for those that take a
	// temperature.
	Count int
	Value float64
}

type controlResponse struct {
	OK bool
}

func (h *Handler) handleHttpControlRequest(_ http.ResponseWriter, r *http.Request, request any) (any, error) {
	if r.Method != http.MethodPost {
		return nil, errors.New("method not allowed")
	}

	body, ok := request.(*controlRequest)

	if !ok || body.Name == "" || body.Action == "" {
		return nil, errors.New("a name and an action are required")
	}

	if err := h.store.Do(body.Name, common.Action(body.Action), body.Count, body.Value); err != nil {
		// What is wrong with the request is for the person who made it, who needs to know. Anything else is not.
		var refused *common.RequestError
		if errors.As(err, &refused) {
			return nil, refused
		}

		h.log.Error("unable to do", "action", body.Action, "name", body.Name, "error", err)

		return nil, errors.New("unable to do that")
	}

	return controlResponse{OK: true}, nil
}

func (h *Handler) devicesRendererFactory() (spi.EntityRenderer, error) {
	compiledTemplate, err := h.container.GetTemplate(&devicesTemplate)

	if err != nil {
		return spi.EntityRenderer{}, fmt.Errorf("unable to load fakes_devices template: %v", err)
	}

	renderer := func(w io.Writer, entity any) error {
		fakes, ok := entity.(*data.Fakes)

		if !ok {
			return errors.New("item is not an instance of *Fakes")
		}

		if err := compiledTemplate.Instance.Execute(w, fakes); err != nil {
			return fmt.Errorf("unable to execute fakes_devices template: %w", err)
		}

		return nil
	}

	return spi.EntityRenderer{
		StreamingRenderFunc: renderer,
		Styles:              compiledTemplate.Styles,
		Scripts:             compiledTemplate.Scripts,
	}, nil
}
