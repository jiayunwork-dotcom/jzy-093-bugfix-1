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
//
// “字段显式给了 0”和“字段压根没给”必须区分开：经 SetMolarity /
// SetMassConcentration / SetMolarMass 赋值（HTTP/登记表路径一律如此）会留下
// 存在性标记，哪怕值是 0 也算给出。直接以字面量构造（包内代码、内置工况档、
// 内核测试）则沿用历史口径——非零即视为给出。
type Solution struct {
	Molarity          float64 // 摩尔浓度 C，单位 mol/L
	MassConcentration float64 // 质量浓度，单位 g/L（可选）
	MolarMass         float64 // 溶质摩尔质量，单位 g/mol（可选）
	Temperature       float64 // 热力学温度 T，单位 K
	VanTHoff          float64 // 范特霍夫因子 i（NaCl 取 2）

	molaritySet          bool // Molarity 是否显式给出（包括显式给 0）
	massConcentrationSet bool // MassConcentration 是否显式给出
	molarMassSet         bool // MolarMass 是否显式给出
}

// SetMolarity 显式设置摩尔浓度并标记该字段已给出（值为 0 也算给出）。
func (s *Solution) SetMolarity(v float64) {
	s.Molarity = v
	s.molaritySet = true
}

// SetMassConcentration 显式设置质量浓度并标记该字段已给出。
func (s *Solution) SetMassConcentration(v float64) {
	s.MassConcentration = v
	s.massConcentrationSet = true
}

// SetMolarMass 显式设置摩尔质量并标记该字段已给出。
func (s *Solution) SetMolarMass(v float64) {
	s.MolarMass = v
	s.molarMassSet = true
}

// HasMolarity 报告摩尔浓度是否显式给出（显式给 0 也是 true）。
func (s Solution) HasMolarity() bool {
	has, _, _ := s.presence()
	return has
}

// HasMassConcentration 报告质量浓度是否显式给出。
func (s Solution) HasMassConcentration() bool {
	_, has, _ := s.presence()
	return has
}

// HasMolarMass 报告摩尔质量是否显式给出。
func (s Solution) HasMolarMass() bool {
	_, _, has := s.presence()
	return has
}

// presence 统一判定三个浓度字段“是否给出”。
// 任一字段走过 SetXxx，就严格以存在性标记为准（请求边界口径，0 也是给出）；
// 三个标记都没置过（包内字面量直接构造）时退化为历史的“非零即给出”口径。
func (s Solution) presence() (hasMolarity, hasMass, hasMolarMass bool) {
	if s.molaritySet || s.massConcentrationSet || s.molarMassSet {
		return s.molaritySet, s.massConcentrationSet, s.molarMassSet
	}
	return s.Molarity != 0, s.MassConcentration != 0, s.MolarMass != 0
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
//
// 字段“显式给出”由存在性标记判定（见 Solution 文档），显式给 0 也算给出，
// 不会被当成没写而悄悄丢掉。口径组合：
//   - 摩尔浓度、质量浓度都给：必须自洽（摩尔质量缺失/非正、质量浓度为负、
//     两者换算不一致都拒绝）；
//   - 只给摩尔浓度：非负即可，允许显式 0（纯水）；
//   - 只给质量浓度或只给摩尔质量：按 ρ/M 换算，摩尔质量必须为正
//     ——质量浓度显式给 0 但没给摩尔质量同样拦下；
//   - 三项都没给：视为纯水 0 mol/L。
func (s Solution) EffectiveMolarity() (float64, error) {
	hasMolarity, hasMass, hasMolarMass := s.presence()
	switch {
	case hasMolarity && hasMass:
		if err := validation.CheckConcentrationConsistency(s.Molarity, s.MassConcentration, s.MolarMass); err != nil {
			return 0, err
		}
		return s.Molarity, nil
	case hasMolarity:
		if err := validation.MolarConcentration(s.Molarity); err != nil {
			return 0, err
		}
		return s.Molarity, nil
	case hasMass || hasMolarMass:
		return FromMassConcentration(s.MassConcentration, s.MolarMass)
	default:
		// 两者都未给：视为纯水 0 mol/L。
		return 0, nil
	}
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
