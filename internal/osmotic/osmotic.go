// Package osmotic 只负责渗透压这一块内核：van't Hoff 关系 π = i·C·R·T，
// 以及质量浓度到摩尔浓度的换算。不涉及任何膜通量或盐衡算逻辑。
package osmotic

import (
	"math"

	"rocalc/internal/validation"
)

// R 为通用气体常数，单位 L·bar/(mol·K)。
// 全服务压力一律用 bar、浓度 mol/L、温度 K，渗透压与压差共用同一量纲，
// 不在两者之间做隐式单位换算。
const R = 0.08314

// Solution 描述进料/产水/浓水一侧的溶液状态。
//
// 摩尔浓度与质量浓度二选一即可；若同时给 Molarity 与 MassConcentration+MolarMass，
// 两者必须自洽（容差见 validation 包），否则拒绝。
type Solution struct {
	Molarity          float64 // 摩尔浓度 C，单位 mol/L
	MassConcentration float64 // 质量浓度，单位 g/L（可选）
	MolarMass         float64 // 溶质摩尔质量，单位 g/mol（可选）
	Temperature       float64 // 热力学温度 T，单位 K
	VanTHoff          float64 // 范特霍夫因子 i（NaCl 取 2）
}

// FromMassConcentration 由质量浓度换算摩尔浓度：C = ρ/M。
// 单位：ρ 为 g/L、M 为 g/mol，结果为 mol/L。
func FromMassConcentration(cMass, molarMass float64) (float64, error) {
	if err := validation.MolarMass(molarMass); err != nil {
		return 0, err
	}
	if err := validation.MassConcentration(cMass); err != nil {
		return 0, err
	}
	return cMass / molarMass, nil
}

// EffectiveMolarity 返回工况采用的摩尔浓度。
// 直接给了 Molarity（>0 或为 0 的纯水）时以它为准；
// 否则用质量浓度除以摩尔质量换算。两种途径同时给时做自洽检查。
func (s Solution) EffectiveMolarity() (float64, error) {
	haveMass := s.MassConcentration != 0 || s.MolarMass != 0
	haveMolar := s.Molarity != 0
	if haveMass && haveMolar {
		if err := validation.CheckConcentrationConsistency(s.Molarity, s.MassConcentration, s.MolarMass); err != nil {
			return 0, err
		}
		return s.Molarity, nil
	}
	if haveMolar {
		if err := validation.MolarConcentration(s.Molarity); err != nil {
			return 0, err
		}
		return s.Molarity, nil
	}
	if haveMass {
		return FromMassConcentration(s.MassConcentration, s.MolarMass)
	}
	// 两者都未给：视为纯水 0 mol/L。
	return 0, nil
}

// Pressure 按 van't Hoff 关系计算渗透压 π = i·C·R·T，单位 bar。
// 温度不为正、因子不为正、浓度为负均拒绝。
func Pressure(s Solution) (float64, error) {
	if err := validation.Temperature(s.Temperature); err != nil {
		return 0, err
	}
	if err := validation.VanTHoff(s.VanTHoff); err != nil {
		return 0, err
	}
	c, err := s.EffectiveMolarity()
	if err != nil {
		return 0, err
	}
	return s.VanTHoff * c * R * s.Temperature, nil
}

// MustPressure 与 Pressure 相同，但把错误向上传递为 panic——仅用于
// 内部测试中“确定合法”的手算对照，业务代码不要使用。
func MustPressure(s Solution) float64 {
	p, err := Pressure(s)
	if err != nil {
		panic(err)
	}
	return p
}

// nearlyZero 保留一个内部小工具，衡算处复用。
func nearlyZero(v, tol float64) bool {
	return math.Abs(v) <= tol
}
