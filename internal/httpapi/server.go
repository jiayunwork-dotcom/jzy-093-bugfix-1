package httpapi

import (
	"encoding/json"
	"net/http"

	"rocalc/internal/cases"
	"rocalc/internal/membrane"
	"rocalc/internal/osmotic"
	"rocalc/internal/validation"
)

// Server 装配登记表与 HTTP 路由。
type Server struct {
	registry *cases.Registry
	mux      *http.ServeMux
}

// NewServer 创建服务并注册内置工况档。
func NewServer() *Server {
	return NewServerWithRegistry(cases.NewDefaultRegistry())
}

// NewServerWithRegistry 使用指定登记表创建服务（测试可注入空表）。
func NewServerWithRegistry(r *cases.Registry) *Server {
	s := &Server{registry: r, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler 暴露根 handler，便于外层套中间件或挂到自定义端口。
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /v1/constants", s.handleConstants)

	s.mux.HandleFunc("GET /v1/cases", s.handleListCases)
	s.mux.HandleFunc("POST /v1/cases", s.handleCreateCase)
	s.mux.HandleFunc("PUT /v1/cases/{name}", s.handleUpsertCase)
	s.mux.HandleFunc("GET /v1/cases/{name}", s.handleGetCase)
	s.mux.HandleFunc("DELETE /v1/cases/{name}", s.handleDeleteCase)
	s.mux.HandleFunc("POST /v1/cases/{name}/evaluate", s.handleEvaluateCase)

	s.mux.HandleFunc("POST /v1/evaluate", s.handleEvaluateAdhoc)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleConstants(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, constantsDTO{
		GasConstant:         osmotic.R,
		OsmoticFormula:      "pi = i*C*R*T  (bar; C in mol/L, T in K)",
		NDFormula:           "NDP = applied_pressure - beta*pi_feed + pi_permeate  (bar)",
		PermeateFormula:     "Qp = area_m2 * Lp_lmh_per_bar * NDP  (L/h)",
		BrineLimitFormula:   "Cb = Cf/(1-Y)   when salt rejection r = 1",
		BrinePartialFormula: "Cb = Cf*(1-(1-r)*Y)/(1-Y); salt: Cf*Qf = Cp*Qp + Cb*Qb",
	})
}

func (s *Server) handleListCases(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"cases": s.registry.List()})
}

func (s *Server) handleCreateCase(w http.ResponseWriter, req *http.Request) {
	dto, ok := decodeSpec(w, req)
	if !ok {
		return
	}
	if dto.Name == "" {
		writeError(w, http.StatusBadRequest, &validation.DomainError{
			Code:   validation.CodeNonPositiveParameter,
			Reason: "登记工况档必须提供 name",
		})
		return
	}
	spec, err := specFromDTO(dto)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := s.registry.Register(spec); err != nil {
		if isErr(err, cases.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, &validation.DomainError{
				Code:   "case_already_exists",
				Reason: err.Error(),
			})
			return
		}
		writeDomainError(w, err)
		return
	}
	w.Header().Set("Location", "/v1/cases/"+spec.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"name": spec.Name, "status": "registered"})
}

func (s *Server) handleUpsertCase(w http.ResponseWriter, req *http.Request) {
	dto, ok := decodeSpec(w, req)
	if !ok {
		return
	}
	name := req.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, &validation.DomainError{
			Code: validation.CodeNonPositiveParameter, Reason: "路径缺少工况档名称",
		})
		return
	}
	dto.Name = name
	spec, err := specFromDTO(dto)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.registry.Put(spec)
	writeJSON(w, http.StatusOK, map[string]any{"name": spec.Name, "status": "stored"})
}

func (s *Server) handleGetCase(w http.ResponseWriter, req *http.Request) {
	spec, err := s.registry.Get(req.PathValue("name"))
	if err != nil {
		if isErr(err, cases.ErrNotFound) {
			writeError(w, http.StatusNotFound, &validation.DomainError{Code: "case_not_found", Reason: err.Error()})
			return
		}
		writeDomainError(w, err)
		return
	}
	dto := specToDTO(spec)
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) handleDeleteCase(w http.ResponseWriter, req *http.Request) {
	name := req.PathValue("name")
	if err := s.registry.Delete(name); err != nil {
		writeError(w, http.StatusNotFound, &validation.DomainError{Code: "case_not_found", Reason: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "status": "deleted"})
}

func (s *Server) handleEvaluateCase(w http.ResponseWriter, req *http.Request) {
	name := req.PathValue("name")
	out, err := s.registry.Evaluate(name)
	if err != nil {
		if isErr(err, cases.ErrNotFound) {
			writeError(w, http.StatusNotFound, &validation.DomainError{Code: "case_not_found", Reason: err.Error()})
			return
		}
		writeDomainError(w, err)
		return
	}
	stored, _ := s.registry.Get(name)
	writeJSON(w, http.StatusOK, outcomeToDTO(name, stored.FeedSpec, out))
}

func (s *Server) handleEvaluateAdhoc(w http.ResponseWriter, req *http.Request) {
	dto, ok := decodeSpec(w, req)
	if !ok {
		return
	}
	spec, err := specFromDTO(dto)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	out, err := membrane.Evaluate(spec.FeedSpec)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, outcomeToDTO(spec.Name, spec.FeedSpec, out))
}

// ---- 编解码与错误映射 ----

func decodeSpec(w http.ResponseWriter, req *http.Request) (specDTO, bool) {
	defer req.Body.Close()
	var dto specDTO
	dec := json.NewDecoder(req.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, &validation.DomainError{
			Code:   "malformed_json",
			Reason: "请求体不是合法 JSON 或含未知字段: " + err.Error(),
		})
		return dto, false
	}
	return dto, true
}

func specFromDTO(dto specDTO) (cases.Spec, error) {
	feed := osmotic.Solution{
		Temperature: dto.Feed.TemperatureK,
		VanTHoff:    dto.Feed.VanTHoff,
	}
	if dto.Feed.Molarity != nil {
		feed.Molarity = *dto.Feed.Molarity
	}
	if dto.Feed.MassConcentration != nil {
		feed.MassConcentration = *dto.Feed.MassConcentration
	}
	if dto.Feed.MolarMass != nil {
		feed.MolarMass = *dto.Feed.MolarMass
	}
	return cases.Spec{
		Name: dto.Name,
		FeedSpec: membrane.FeedSpec{
			Feed:            feed,
			AppliedPressure: dto.AppliedPressureBar,
			FeedFlow:        dto.FeedFlowLh,
			Permeability:    dto.PermeabilityLMH,
			Area:            dto.AreaM2,
			Rejection:       dto.Rejection,
			Polarization:    dto.Polarization,
		},
	}, nil
}

func specToDTO(spec cases.Spec) specDTO {
	return specDTO{
		Name:               spec.Name,
		Feed:               toSolutionDTO(spec.Feed),
		AppliedPressureBar: spec.AppliedPressure,
		FeedFlowLh:         spec.FeedFlow,
		PermeabilityLMH:    spec.Permeability,
		AreaM2:             spec.Area,
		Rejection:          spec.Rejection,
		Polarization:       spec.Polarization,
	}
}

func outcomeToDTO(name string, spec membrane.FeedSpec, out *membrane.Outcome) outcomeDTO {
	dto := outcomeDTO{
		Name:                       name,
		FeedMolarityMolar:          out.FeedMolarity,
		PermeateMolarityMolar:      out.PermeateMolarity,
		BrineMolarityMolar:         out.BrineMolarity,
		FeedOsmoticPressureBar:     out.FeedOsmoticPressure,
		WallOsmoticPressureBar:     out.WallOsmoticPressure,
		PermeateOsmoticPressureBar: out.PermeateOsmoticPressure,
		NetDrivingPressureBar:      out.NetDrivingPressure,
		PermeateFlowLh:             out.PermeateFlow,
		BrineFlowLh:                out.BrineFlow,
		Recovery:                   out.Recovery,
		Salt: saltFlowsDTO{
			FeedSaltMassFlowGh:     out.Salt.FeedSaltMassFlow,
			PermeateSaltMassFlowGh: out.Salt.PermeateSaltMassFlow,
			BrineSaltMassFlowGh:    out.Salt.BrineSaltMassFlow,
			ResidualGh:             out.Salt.Residual,
		},
		Units: standardUnits(),
	}
	if spec.Feed.MolarMass > 0 {
		m := spec.Feed.MolarMass
		dto.FeedMassConcentration = out.FeedMolarity * m
		dto.BrineMassConcentration = out.BrineMolarity * m
	}
	return dto
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, e *validation.DomainError) {
	writeJSON(w, status, errorDTO{Error: e.Error(), Code: e.Code, Reason: e.Reason})
}

// writeDomainError 把内核领域错误映射成合适的 HTTP 状态码：
// 工况不合法/不物理一律 400，并把具体原因带回去。
func writeDomainError(w http.ResponseWriter, err error) {
	if de, ok := err.(*validation.DomainError); ok {
		writeError(w, http.StatusBadRequest, de)
		return
	}
	writeError(w, http.StatusInternalServerError, &validation.DomainError{
		Code:   "internal_error",
		Reason: err.Error(),
	})
}

func isErr(err, target error) bool {
	return err != nil && (err == target || isUnwrap(err, target))
}

func isUnwrap(err, target error) bool {
	type unwrapper interface{ Unwrap() error }
	for {
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
		if err == nil {
			return false
		}
		if err == target {
			return true
		}
	}
}
