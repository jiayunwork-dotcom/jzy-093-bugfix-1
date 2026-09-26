package membrane

import (
	"fmt"

	"rocalc/internal/osmotic"
	"rocalc/internal/validation"
)

// FullRejectionEps 是“盐截留率贴近一”的判定阈值。
// r >= 1−eps 时按完全截留处理：产水零盐、盐全进浓水，
// 浓水浓度走 Cb = Cf/(1−Y) 这个极限。
const FullRejectionEps = 1e-9

// NetDrivingPressure 计算净推动力：
//
//	NDP = Δp − β·πf + πp
//
// 其中 β·πf 是浓差极化抬高后的膜面渗透压，πp 是产水侧渗透压（带盐时非零）。
// 各压力项都已统一为 bar，函数内部不做任何量纲换算。
func NetDrivingPressure(applied, feedOsmotic, permeateOsmotic, beta float64) float64 {
	return applied - beta*feedOsmotic + permeateOsmotic
}

// PermeateFlow 按 Qp = A·Lp·NDP 计算产水流量，L/h。
// Lp 单位 LMH = L/(m²·h)/bar，A 单位 m²，因此 A·Lp·NDP 单位为 L/h。
func PermeateFlow(area, permeability, ndp float64) float64 {
	return area * permeability * ndp
}

// Evaluate 对一份完整工况执行核算并做全套合法性与守恒校验。
//
// 拒绝条件（带原因返回 *validation.DomainError）：
//   - 温度不为正、范特霍夫因子不为正、浓度为负；
//   - 工作压差、进料流量、膜面积、渗透系数不为正；
//   - 极化因子 < 1、截留率不在 (0,1]；
//   - NDP < 0（工作压差已低于膜面渗透压却还要出正水，物理上非法）；
//   - 回收率 Y 不在开区间 (0,1)；
//   - 进料盐量 ≠ 产水盐量 + 浓水盐量（守恒被破坏）。
func Evaluate(spec FeedSpec) (*Outcome, error) {
	// ---- 默认值 ----
	beta := spec.Polarization
	if beta == 0 {
		beta = 1
	}

	// ---- 输入合法性 ----
	if err := validation.Temperature(spec.Feed.Temperature); err != nil {
		return nil, err
	}
	if err := validation.VanTHoff(spec.Feed.VanTHoff); err != nil {
		return nil, err
	}
	cf, err := spec.Feed.EffectiveMolarity()
	if err != nil {
		return nil, err
	}
	if err := validation.Positive(spec.AppliedPressure, "工作压差"); err != nil {
		return nil, err
	}
	if err := validation.Positive(spec.FeedFlow, "进料流量"); err != nil {
		return nil, err
	}
	if err := validation.Positive(spec.Area, "膜面积"); err != nil {
		return nil, err
	}
	if err := validation.Positive(spec.Permeability, "渗透系数"); err != nil {
		return nil, err
	}
	if err := validation.Rejection(spec.Rejection); err != nil {
		return nil, err
	}
	if err := validation.Polarization(beta); err != nil {
		return nil, err
	}

	// ---- 渗透压（全程 bar）----
	piF, err := osmotic.Pressure(spec.Feed)
	if err != nil {
		return nil, err
	}

	cp, err := PermeateMolarity(cf, spec.Rejection)
	if err != nil {
		return nil, err
	}
	piP := 0.0
	if cp > 0 {
		piP, err = osmotic.Pressure(osmotic.Solution{
			Molarity:    cp,
			Temperature: spec.Feed.Temperature,
			VanTHoff:    spec.Feed.VanTHoff,
		})
		if err != nil {
			return nil, err
		}
	}
	piWall := beta * piF

	// ---- NDP 与通量 ----
	ndp := NetDrivingPressure(spec.AppliedPressure, piF, piP, beta)
	if ndp < 0 {
		return nil, validation.NewError(validation.CodeNonPositiveNDP,
			fmt.Sprintf("工作压差 %g bar 低于膜面渗透压 %g bar（β=%g），NDP=%g bar，无法产水",
				spec.AppliedPressure, piWall, beta, ndp))
	}
	qp := PermeateFlow(spec.Area, spec.Permeability, ndp)
	if ndp == 0 && qp > 0 {
		// 由公式本身不可能触发；保留这道防线，明确“零推动力不得有正通量”。
		return nil, validation.NewError(validation.CodeNonPositiveNDP,
			"NDP 为 0 却出现正产水通量")
	}

	y := qp / spec.FeedFlow
	if err := validation.Recovery(y); err != nil {
		return nil, err
	}
	qb := spec.FeedFlow - qp

	// ---- 浓水浓度与盐衡算 ----
	cb, err := BrineMolarity(cf, y, spec.Rejection)
	if err != nil {
		return nil, err
	}

	out := &Outcome{
		FeedMolarity:            cf,
		PermeateMolarity:        cp,
		BrineMolarity:           cb,
		FeedOsmoticPressure:     piF,
		WallOsmoticPressure:     piWall,
		PermeateOsmoticPressure: piP,
		NetDrivingPressure:      ndp,
		PermeateFlow:            qp,
		BrineFlow:               qb,
		Recovery:                y,
	}

	// ---- 全系统物料守恒（摩尔口径；给了摩尔质量再校质量口径）----
	if err := CheckBalance(out, spec); err != nil {
		return nil, err
	}
	return out, nil
}
