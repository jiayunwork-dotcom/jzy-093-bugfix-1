package cases

import (
	"rocalc/internal/membrane"
	"rocalc/internal/osmotic"
)

// solution 用质量浓度口径构造一份溶液。
func solution(massConcentration, molarMass, temperature, vanTHoff float64) osmotic.Solution {
	return osmotic.Solution{
		MassConcentration: massConcentration,
		MolarMass:         molarMass,
		Temperature:       temperature,
		VanTHoff:          vanTHoff,
	}
}

// DefaultBrackishCase 是内置苦咸水工况档的名称。
const DefaultBrackishCase = "brackish-default"

// BuiltinBrackishSpec 内置的小型苦咸水工况（以 NaCl 计）。
//
// 进料 5 g/L NaCl（约 5000 mg/L，苦咸水常见量级），25 ℃（298.15 K），
// i=2。渗透压 π = 2 × (5/58.44) × 0.08314 × 298.15 ≈ 4.24 bar，
// 落在“几个巴”的量级，显著低于海水的二十多巴。
//
// 工作压差取 12 bar，膜面积 36 m²，Lp = 1.8 LMH/bar，
// 进料 1000 L/h，完全截留（r=1），忽略浓差极化（β=1）。
func BuiltinBrackishSpec() Spec {
	return Spec{
		Name: DefaultBrackishCase,
		FeedSpec: membrane.FeedSpec{
			Feed: solution(
				5.0,    // 质量浓度 g/L
				58.44,  // NaCl 摩尔质量 g/mol
				298.15, // 温度 K
				2,      // van't Hoff 因子
			),
			AppliedPressure: 12.0, // bar
			FeedFlow:        1000, // L/h
			Permeability:    1.8,  // LMH/bar
			Area:            36,   // m²
			Rejection:       1,    // 完全截留
			Polarization:    1,    // 无极化
		},
	}
}

// NewDefaultRegistry 建表并登记内置工况档。
func NewDefaultRegistry() *Registry {
	r := NewRegistry()
	r.Put(BuiltinBrackishSpec())
	return r
}
