// Package membrane 负责膜那一侧的过程核算：净推动力 NDP、产水通量、
// 回收率、浓水浓度与全系统盐衡算。
//
// 通量/衡算逻辑拆在 flux.go 与 balance.go 两个文件里：
//   - flux.go：NDP = Δp − β·πf + πp、Qp = A·Lp·NDP、回收率
//   - balance.go：浓水浓度、产水带盐、进料 = 产水 + 浓水的物料守恒
//
// 全程压力单位统一为 bar，流量 L/h，膜面积 m²，渗透系数 LMH = L/(m²·h)。
package membrane

import (
	"rocalc/internal/osmotic"
)

// FeedSpec 是一次膜核算需要的完整工况输入（一份“工况档”的内核）。
type FeedSpec struct {
	// 进料溶液（mol/L 或 g/L + g/mol 二选一或自洽地同时给）。
	Feed osmotic.Solution

	// AppliedPressure 是膜两侧施加的工作压差 Δp（进料侧压力减产水侧背压），bar。
	AppliedPressure float64

	// FeedFlow 进料流量 Qf，L/h。
	FeedFlow float64

	// Permeability 膜渗透系数 Lp，单位 LMH = L/(m²·h)/bar。
	Permeability float64

	// Area 膜面积 A，m²。
	Area float64

	// Rejection 盐截留率 r，落在 (0,1]。1 表示完全截留、产水零盐。
	Rejection float64

	// Polarization 浓差极化因子 β，>= 1；零值按 1（无极化）处理。
	Polarization float64
}

// SaltFlows 是系统三股流的盐流量与盐浓度，用于物料守恒展示与校验。
type SaltFlows struct {
	FeedSaltMassFlow     float64 // 进料盐质量流量，g/h
	PermeateSaltMassFlow float64 // 产水盐质量流量，g/h
	BrineSaltMassFlow    float64 // 浓水盐质量流量，g/h
	Residual             float64 // 守恒残差 = 进料 −（产水 + 浓水），g/h
}

// Outcome 是一次膜核算的全部结果。
type Outcome struct {
	FeedMolarity     float64 // 进料摩尔浓度，mol/L
	PermeateMolarity float64 // 产水摩尔浓度，mol/L
	BrineMolarity    float64 // 浓水摩尔浓度，mol/L

	FeedOsmoticPressure     float64 // 进料渗透压 πf，bar
	WallOsmoticPressure     float64 // 膜面渗透压 β·πf，bar
	PermeateOsmoticPressure float64 // 产水渗透压 πp，bar

	NetDrivingPressure float64 // NDP，bar
	PermeateFlow       float64 // Qp，L/h
	BrineFlow          float64 // Qb = Qf − Qp，L/h
	Recovery           float64 // Y = Qp/Qf

	Salt SaltFlows
}
