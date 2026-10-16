package http

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"reflect"
	"time"

	"github.com/avanha/pmaas-plugin-fakes/data"
	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	"github.com/avanha/pmaas-spi"
)

//go:embed content/static content/templates
var contentFS embed.FS

var switchesTemplate = spi.TemplateInfo{
	Name: "fakes_switches",
	FuncMap: template.FuncMap{
		"RelativeTime": relativeTime,
	},
	Paths:   []string{"templates/fakes_switches.htmlt"},
	Styles:  []string{"css/fakes_switches.css"},
	Scripts: []string{"js/fakes_switches.js"},
}

// Handler owns the plugin's HTTP surface: the page that shows the fakes, and the route that changes them.
type Handler struct {
	container spi.IPMAASContainer
	store     common.SwitchStore
}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) Init(container spi.IPMAASContainer, store common.SwitchStore) {
	h.container = container
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
		reflect.TypeOf((*data.Switches)(nil)).Elem(),
		h.switchesRendererFactory)
}

func (h *Handler) handleHttpListRequest(w http.ResponseWriter, r *http.Request) {
	switches, err := h.store.GetSwitches()

	if err != nil {
		fmt.Printf("fakes.http handleHttpListRequest: unable to retrieve the fakes: %v\n", err)
		switches = data.Switches{}
	}

	h.container.RenderList(w, r, spi.RenderListOptions{Title: "Fakes"}, []any{&switches})
}

type controlRequest struct {
	Name   string
	Action string
	Count  int
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

	if err := h.store.Do(body.Name, common.Action(body.Action), body.Count); err != nil {
		// What is wrong with the request is for the person who made it, who needs to know. Anything else is not.
		var refused *common.RequestError
		if errors.As(err, &refused) {
			return nil, refused
		}

		fmt.Printf("fakes.http: unable to do %q to %q: %v\n", body.Action, body.Name, err)

		return nil, errors.New("unable to do that")
	}

	return controlResponse{OK: true}, nil
}

func (h *Handler) switchesRendererFactory() (spi.EntityRenderer, error) {
	compiledTemplate, err := h.container.GetTemplate(&switchesTemplate)

	if err != nil {
		return spi.EntityRenderer{}, fmt.Errorf("unable to load fakes_switches template: %v", err)
	}

	renderer := func(w io.Writer, entity any) error {
		switches, ok := entity.(*data.Switches)

		if !ok {
			return errors.New("item is not an instance of *Switches")
		}

		if err := compiledTemplate.Instance.Execute(w, switches); err != nil {
			return fmt.Errorf("unable to execute fakes_switches template: %w", err)
		}

		return nil
	}

	return spi.EntityRenderer{
		StreamingRenderFunc: renderer,
		Styles:              compiledTemplate.Styles,
		Scripts:             compiledTemplate.Scripts,
	}, nil
}

// relativeTime says how long ago a time was, in its two most significant units: "5m ago", "2h 5m ago",
// "3d 4h ago". A time that is zero reads as "never".
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}

	elapsed := time.Since(t).Truncate(time.Minute)

	switch {
	case elapsed < time.Minute:
		return "just now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm ago", int(elapsed.Minutes()))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh %dm ago", int(elapsed.Hours()), int(elapsed.Minutes())%60)
	default:
		return fmt.Sprintf("%dd %dh ago", int(elapsed.Hours())/24, int(elapsed.Hours())%24)
	}
}
