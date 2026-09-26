package osmotic

import (
	"math"
	"testing"
)

func TestPressure_NaClHandCalc(t *testing.T) {
	// 0.1 mol/L NaCl，i=2，25 ℃：
	// π = 2 × 0.1 × 0.08314 × 298.15 ≈ 4.958 bar
	s := Solution{Molarity: 0.1, Temperature: 298.15, VanTHoff: 2}
	got, err := Pressure(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const want = 2 * 0.1 * R * 298.15
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("π = %.6f bar, 手算 %.6f bar", got, want)
	}
}

func TestPressure_BrackishOrderOfMagnitude(t *testing.T) {
	// 5 g/L NaCl（苦咸水）：渗透压应落在“几个巴”量级。
	s := Solution{MassConcentration: 5, MolarMass: 58.44, Temperature: 298.15, VanTHoff: 2}
	got, err := Pressure(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got < 2 || got > 8 {
		t.Fatalf("苦咸水渗透压应在几个巴量级，实际 %.4f bar", got)
	}
	t.Logf("5 g/L NaCl 渗透压 = %.4f bar", got)
}

func TestPressure_RisesWithTemperature(t *testing.T) {
	base := Solution{Molarity: 0.2, Temperature: 290, VanTHoff: 2}
	hot := base
	hot.Temperature = 310
	pLow, err := Pressure(base)
	if err != nil {
		t.Fatal(err)
	}
	pHigh, err := Pressure(hot)
	if err != nil {
		t.Fatal(err)
	}
	if !(pHigh > pLow) {
		t.Fatalf("升温渗透压必须上升：%.4f -> %.4f", pLow, pHigh)
	}
}

func TestPressure_RejectsInvalidInputs(t *testing.T) {
	cases := []struct {
		name string
		sol  Solution
		code string
	}{
		{"温度为零", Solution{Molarity: 0.1, Temperature: 0, VanTHoff: 2}, "invalid_temperature"},
		{"温度为负", Solution{Molarity: 0.1, Temperature: -273.15, VanTHoff: 2}, "invalid_temperature"},
		{"因子为零", Solution{Molarity: 0.1, Temperature: 298.15, VanTHoff: 0}, "invalid_vanthoff_factor"},
		{"因子为负", Solution{Molarity: 0.1, Temperature: 298.15, VanTHoff: -2}, "invalid_vanthoff_factor"},
		{"浓度为负", Solution{Molarity: -0.1, Temperature: 298.15, VanTHoff: 2}, "negative_concentration"},
		{"质量浓度为负", Solution{MassConcentration: -1, MolarMass: 58.44, Temperature: 298.15, VanTHoff: 2}, "negative_concentration"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Pressure(tc.sol); err == nil {
				t.Fatalf("期望错误码 %s，实际通过", tc.code)
			}
		})
	}
}

func TestEffectiveMolarity_MassAndMolarConsistency(t *testing.T) {
	// 5 g/L ÷ 58.44 g/mol ≈ 0.085558 mol/L，与直接给的值自洽。
	s := Solution{Molarity: 5.0 / 58.44, MassConcentration: 5.0, MolarMass: 58.44,
		Temperature: 298.15, VanTHoff: 2}
	c, err := s.EffectiveMolarity()
	if err != nil {
		t.Fatalf("自洽输入不应报错: %v", err)
	}
	if math.Abs(c-5.0/58.44) > 1e-12 {
		t.Fatalf("摩尔浓度 %v 不等于换算值", c)
	}

	// 两种口径明显打架时必须拒绝。
	bad := Solution{Molarity: 0.5, MassConcentration: 5.0, MolarMass: 58.44,
		Temperature: 298.15, VanTHoff: 2}
	if _, err := bad.EffectiveMolarity(); err == nil {
		t.Fatal("摩尔浓度与质量浓度换算不自洽时必须拒绝")
	}
}
