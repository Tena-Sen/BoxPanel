package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"boxpanel/internal/models"
)

// ----- groups -----

func (s *APIServer) handleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, _ := s.store.ListGroups(r.Context())
	if groups == nil {
		groups = []models.Group{}
	}
	writeJSON(w, 200, groups)
}

func (s *APIServer) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var g models.Group
	if err := readJSON(r, &g); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if g.ID == "" {
		g.ID = models.NewID("grp")
	}
	if g.Type == "" {
		g.Type = models.GroupSelector
	}
	if g.Name == "" {
		g.Name = "新分组"
	}
	_ = s.store.SaveGroup(r.Context(), g)
	writeJSON(w, 200, g)
}

func (s *APIServer) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	g := mustBody(models.Group{}, r, w)
	if g == nil {
		return
	}
	g.ID = chi.URLParam(r, "id")
	_ = s.store.SaveGroup(r.Context(), *g)
	writeJSON(w, 200, g)
}

func (s *APIServer) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	_ = s.store.DeleteGroup(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"deleted": chi.URLParam(r, "id")})
}

// ----- subscriptions -----

func (s *APIServer) handleListSubs(w http.ResponseWriter, r *http.Request) {
	subs, _ := s.store.ListSubscriptions(r.Context())
	if subs == nil {
		subs = []models.Subscription{}
	}
	writeJSON(w, 200, subs)
}

func (s *APIServer) handleCreateSub(w http.ResponseWriter, r *http.Request) {
	var sub models.Subscription
	if err := readJSON(r, &sub); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if sub.ID == "" {
		sub.ID = models.NewID("sub")
	}
	if sub.IntervalHours == 0 {
		sub.IntervalHours = 24
	}
	_ = s.store.SaveSubscription(r.Context(), sub)
	writeJSON(w, 200, sub)
}

func (s *APIServer) handleUpdateSub(w http.ResponseWriter, r *http.Request) {
	sub := mustBody(models.Subscription{}, r, w)
	if sub == nil {
		return
	}
	sub.ID = chi.URLParam(r, "id")
	_ = s.store.SaveSubscription(r.Context(), *sub)
	writeJSON(w, 200, sub)
}

func (s *APIServer) handleDeleteSub(w http.ResponseWriter, r *http.Request) {
	_ = s.store.DeleteSubscription(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"deleted": chi.URLParam(r, "id")})
}

func (s *APIServer) handleRefreshSub(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Replace bool `json:"replace"`
	}
	_ = readJSON(r, &body)
	res, err := s.subs.Refresh(r.Context(), chi.URLParam(r, "id"), body.Replace)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

// ----- profiles -----

func (s *APIServer) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	profs, _ := s.store.ListProfiles(r.Context())
	if profs == nil {
		profs = []models.Profile{}
	}
	writeJSON(w, 200, profs)
}

func (s *APIServer) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var p models.Profile
	if err := readJSON(r, &p); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if p.ID == "" {
		p.ID = models.NewID("prof")
	}
	if p.Mode == "" {
		p.Mode = "normal"
	}
	if p.ListenPort == 0 {
		p.ListenPort = 20808
	}
	_ = s.store.SaveProfile(r.Context(), p)
	writeJSON(w, 200, p)
}

func (s *APIServer) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	p := mustBody(models.Profile{}, r, w)
	if p == nil {
		return
	}
	p.ID = chi.URLParam(r, "id")
	_ = s.store.SaveProfile(r.Context(), *p)
	writeJSON(w, 200, p)
}

func (s *APIServer) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	_ = s.store.DeleteProfile(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"deleted": chi.URLParam(r, "id")})
}

// mustBody decodes JSON body into a new T; on error writes 400 and returns nil.
func mustBody[T any](zero T, r *http.Request, w http.ResponseWriter) *T {
	v := new(T)
	if err := readJSON(r, v); err != nil {
		writeError(w, 400, err.Error())
		return nil
	}
	return v
}
