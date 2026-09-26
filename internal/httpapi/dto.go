// Package httpapi 用标准库 net/http 暴露反渗透核算服务。
// 服务只提供 JSON over HTTP，不做页面。
package httpapi

import "rocalc/internal/osmotic"

// 单位约定（随文档与字段名一并给出，避免量纲混淆）：
//   - 压力（渗透压、工作压差、背压、NDP）：bar
//   - 摩尔浓度：mol/L；质量浓度：g/L；摩尔质量：g/mol
//   - 温度：K；流量：L/h；膜面积：m²；渗透系数 Lp：LMH = L/(m²·h)/bar
type feedSolutionDTO struct {
	Molarity          *float64 `json:"molarity_mol_per_l,omitempty"`
	MassConcentration *float64 `json:"mass_concentration_g_per_l,omitempty"`
	MolarMass         *float64 `json:"molar_mass_g_per_mol,omitempty"`
	TemperatureK      float64  `json:"temperature_k"`
	VanTHoff          float64  `json:"vanth_hoff_factor"`
}

type specDTO struct {
	Name string `json:"name,omitempty"`

	Feed feedSolutionDTO `json:"feed"`

	AppliedPressureBar float64 `json:"applied_pressure_bar"`
	FeedFlowLh         float64 `json:"feed_flow_lh"`
	PermeabilityLMH    float64 `json:"permeability_lmh_per_bar"`
	AreaM2             float64 `json:"area_m2"`
	Rejection          float64 `json:"salt_rejection"`
	Polarization       float64 `json:"polarization_factor,omitempty"`
}

type saltFlowsDTO struct {
	FeedSaltMassFlowGh     float64 `json:"feed_salt_mass_flow_gh"`
	PermeateSaltMassFlowGh float64 `json:"permeate_salt_mass_flow_gh"`
	BrineSaltMassFlowGh    float64 `json:"brine_salt_mass_flow_gh"`
	ResidualGh             float64 `json:"residual_gh"`
}

type outcomeDTO struct {
	Name string `json:"name,omitempty"`

	FeedMolarityMolar      float64 `json:"feed_molarity_mol_per_l"`
	PermeateMolarityMolar  float64 `json:"permeate_molarity_mol_per_l"`
	BrineMolarityMolar     float64 `json:"brine_molarity_mol_per_l"`
	FeedMassConcentration  float64 `json:"feed_mass_concentration_g_per_l,omitempty"`
	BrineMassConcentration float64 `json:"brine_mass_concentration_g_per_l,omitempty"`

	FeedOsmoticPressureBar     float64 `json:"feed_osmotic_pressure_bar"`
	WallOsmoticPressureBar     float64 `json:"wall_osmotic_pressure_bar"`
	PermeateOsmoticPressureBar float64 `json:"permeate_osmotic_pressure_bar"`

	NetDrivingPressureBar float64 `json:"net_driving_pressure_bar"`
	PermeateFlowLh        float64 `json:"permeate_flow_lh"`
	BrineFlowLh           float64 `json:"brine_flow_lh"`
	Recovery              float64 `json:"recovery"`

	Salt saltFlowsDTO `json:"salt_balance"`

	Units unitsDTO `json:"units"`
}

type unitsDTO struct {
	Pressure     string `json:"pressure"`
	Molarity     string `json:"molarity"`
	MassConc     string `json:"mass_concentration"`
	Temperature  string `json:"temperature"`
	Flow         string `json:"flow"`
	Area         string `json:"area"`
	Permeability string `json:"permeability"`
}

func standardUnits() unitsDTO {
	return unitsDTO{
		Pressure:     "bar",
		Molarity:     "mol/L",
		MassConc:     "g/L",
		Temperature:  "K",
		Flow:         "L/h",
		Area:         "m^2",
		Permeability: "LMH = L/(m^2*h)/bar",
	}
}

type constantsDTO struct {
	GasConstant         float64 `json:"gas_constant_r_bar_l_mol_k"`
	OsmoticFormula      string  `json:"osmotic_formula"`
	NDFormula           string  `json:"ndp_formula"`
	PermeateFormula     string  `json:"permeate_flow_formula"`
	BrineLimitFormula   string  `json:"brine_full_rejection_formula"`
	BrinePartialFormula string  `json:"brine_partial_rejection_formula"`
}

type errorDTO struct {
	Error  string `json:"error"`
	Code   string `json:"code,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func toSolutionDTO(s osmotic.Solution) feedSolutionDTO {
	dto := feedSolutionDTO{
		TemperatureK: s.Temperature,
		VanTHoff:     s.VanTHoff,
	}
	if s.Molarity != 0 {
		v := s.Molarity
		dto.Molarity = &v
	}
	if s.MassConcentration != 0 {
		v := s.MassConcentration
		dto.MassConcentration = &v
	}
	if s.MolarMass != 0 {
		v := s.MolarMass
		dto.MolarMass = &v
	}
	return dto
}
