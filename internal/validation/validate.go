package validation

import (
	"fmt"
	"math"
)

// 浓度自洽容差：直接给出的摩尔浓度与由质量浓度换算得到的摩尔浓度，
// 相对偏差不超过 0.1%；浓度接近 0 时退化为绝对容差 1e-6 mol/L。
const (
	ConcentrationRelTol = 1e-3
	concentrationAbsTol = 1e-6
)

// Positive 要求 v 严格为正。
func Positive(v float64, field string) error {
	if v <= 0 {
		return NewError(CodeNonPositiveParameter,
			fmt.Sprintf("%s 必须为正，实际为 %g", field, v))
	}
	return nil
}

// NonNegative 要求 v 非负。
func NonNegative(v float64, field string) error {
	if v < 0 {
		return NewError(CodeNonPositiveParameter,
			fmt.Sprintf("%s 不能为负，实际为 %g", field, v))
	}
	return nil
}

// Temperature 校验开尔文温度严格为正。
func Temperature(t float64) error {
	if t <= 0 {
		return NewError(CodeInvalidTemperature,
			fmt.Sprintf("温度必须为正（开尔文），实际为 %g K", t))
	}
	return nil
}

// MolarConcentration 校验摩尔浓度非负（允许纯水 0 mol/L）。
func MolarConcentration(c float64) error {
	if c < 0 {
		return NewError(CodeNegativeConcentration,
			fmt.Sprintf("摩尔浓度不能为负，实际为 %g mol/L", c))
	}
	return nil
}

// MassConcentration 校验质量浓度非负。
func MassConcentration(c float64) error {
	if c < 0 {
		return NewError(CodeNegativeConcentration,
			fmt.Sprintf("质量浓度不能为负，实际为 %g g/L", c))
	}
	return nil
}

// VanTHoff 校验范特霍夫因子严格为正。
func VanTHoff(i float64) error {
	if i <= 0 {
		return NewError(CodeInvalidVanTHoff,
			fmt.Sprintf("范特霍夫因子必须为正，实际为 %g", i))
	}
	return nil
}

// MolarMass 校验摩尔质量严格为正（用于质量浓度换算）。
func MolarMass(m float64) error {
	if m <= 0 {
		return NewError(CodeNonPositiveParameter,
			fmt.Sprintf("摩尔质量必须为正，实际为 %g g/mol", m))
	}
	return nil
}

// Rejection 校验盐截留率落在半开区间 (0,1]。
func Rejection(r float64) error {
	if r <= 0 || r > 1 {
		return NewError(CodeInvalidRejection,
			fmt.Sprintf("盐截留率必须落在 (0,1]，实际为 %g", r))
	}
	return nil
}

// Recovery 校验回收率落在开区间 (0,1)。
func Recovery(y float64) error {
	if y <= 0 || y >= 1 {
		return NewError(CodeInvalidRecovery,
			fmt.Sprintf("回收率必须落在开区间 (0,1)，实际为 %g", y))
	}
	return nil
}

// Polarization 校验浓差极化因子 beta >= 1（1 表示忽略极化）。
func Polarization(beta float64) error {
	if beta < 1 {
		return NewError(CodeInvalidPolarization,
			fmt.Sprintf("浓差极化因子必须 >= 1（1 表示无极化），实际为 %g", beta))
	}
	return nil
}

// CheckConcentrationConsistency 校验“直接给的摩尔浓度”与
// “质量浓度 / 摩尔质量”换算出的摩尔浓度互相自洽。
// cMolar 单位 mol/L；cMass 单位 g/L；molarMass 单位 g/mol。
func CheckConcentrationConsistency(cMolar, cMass, molarMass float64) error {
	if err := MolarMass(molarMass); err != nil {
		return err
	}
	if err := MassConcentration(cMass); err != nil {
		return err
	}
	derived := cMass / molarMass
	tol := ConcentrationRelTol * math.Max(math.Abs(cMolar), math.Abs(derived))
	if tol < concentrationAbsTol {
		tol = concentrationAbsTol
	}
	if math.Abs(cMolar-derived) > tol {
		return NewError(CodeInconsistentConcentration,
			fmt.Sprintf("摩尔浓度不自洽：直接给定 %g mol/L，由质量浓度换算为 %g mol/L（%g g/L ÷ %g g/mol）",
				cMolar, derived, cMass, molarMass))
	}
	return nil
}
