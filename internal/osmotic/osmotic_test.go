package osmotic

import (
	"errors"
	"math"
	"strings"
	"testing"

	"rocalc/internal/validation"
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

// codeFrom 返回错误携带的稳定错误码，非领域错误返回空串。
func codeFrom(err error) string {
	var de *validation.DomainError
	if errors.As(err, &de) {
		return de.Code
	}
	return ""
}

// TestEffectiveMolarity_ExplicitZeroIsPresent 钉死“显式给 0”和“压根没给”的区别。
// 经 SetXxx 进入（对应 HTTP/登记表路径）的字段，0 也是给出，内核不得替调用方
// 悄悄挑另一边。
func TestEffectiveMolarity_ExplicitZeroIsPresent(t *testing.T) {
	// 显式摩尔浓度 0 撞上非零质量浓度：与给 0.05 一样判 inconsistent_concentration，
	// 不能把 0 当没写、转而采用质量口径。
	var conflicting Solution
	conflicting.SetMolarity(0)
	conflicting.SetMassConcentration(5)
	conflicting.SetMolarMass(58.44)
	c, err := conflicting.EffectiveMolarity()
	if codeFrom(err) != validation.CodeInconsistentConcentration {
		t.Fatalf("显式摩尔浓度 0 与非零质量浓度冲突应报 %s，实际 c=%g err=%v",
			validation.CodeInconsistentConcentration, c, err)
	}
	if got := err.Error(); !containsAll(got, "0", "0.0855", "5", "58.44") {
		t.Fatalf("拒绝原因应同时带上两边数值（摩尔 0、换算值、质量浓度、摩尔质量），实际：%s", got)
	}

	// 纯水写法一：只带摩尔浓度 0，其余浓度项都不带。
	var pureMolar Solution
	pureMolar.SetMolarity(0)
	if c, err := pureMolar.EffectiveMolarity(); err != nil || c != 0 {
		t.Fatalf("只给摩尔浓度 0 应按纯水 0 mol/L 出结果，实际 c=%g err=%v", c, err)
	}

	// 纯水写法二：三项都带，但摩尔浓度 0、质量浓度 0、摩尔质量为正。
	var pureBoth Solution
	pureBoth.SetMolarity(0)
	pureBoth.SetMassConcentration(0)
	pureBoth.SetMolarMass(58.44)
	if c, err := pureBoth.EffectiveMolarity(); err != nil || c != 0 {
		t.Fatalf("两边都是 0（摩尔质量为正）应按纯水出结果，实际 c=%g err=%v", c, err)
	}

	// 质量浓度显式给 0 却没给摩尔质量：与给非零质量浓度一样拦下，
	// 不能因为数值是 0 就把这个字段当没写、再按摩尔口径放行。
	var zeroMassNoMMass Solution
	zeroMassNoMMass.SetMolarity(0.1)
	zeroMassNoMMass.SetMassConcentration(0)
	if _, err := zeroMassNoMMass.EffectiveMolarity(); codeFrom(err) != validation.CodeNonPositiveParameter {
		t.Fatalf("显式质量浓度 0 缺摩尔质量应报 %s，实际 err=%v",
			validation.CodeNonPositiveParameter, err)
	}
	var massOnlyZero Solution
	massOnlyZero.SetMassConcentration(0)
	if _, err := massOnlyZero.EffectiveMolarity(); codeFrom(err) != validation.CodeNonPositiveParameter {
		t.Fatalf("只给质量浓度 0、不给摩尔质量也应报 %s，实际 err=%v",
			validation.CodeNonPositiveParameter, err)
	}

	// 自洽地同时给（5 g/L ÷ 58.44 ≈ 0.08556）照常放行，数值一位不变。
	var consistent Solution
	consistent.SetMolarity(5.0 / 58.44)
	consistent.SetMassConcentration(5.0)
	consistent.SetMolarMass(58.44)
	if c, err := consistent.EffectiveMolarity(); err != nil || math.Abs(c-5.0/58.44) > 1e-12 {
		t.Fatalf("自洽口径应照常采用摩尔浓度，实际 c=%g err=%v", c, err)
	}
}

// TestSolution_PresenceAccessors 确认存在性标记与 GET 回显依据的语义：
// 显式给 0 的字段 Has* 为 true；没给的为 false。
func TestSolution_PresenceAccessors(t *testing.T) {
	var s Solution
	s.SetMolarity(0)
	s.SetMassConcentration(0)
	s.SetMolarMass(58.44)
	if !s.HasMolarity() || !s.HasMassConcentration() || !s.HasMolarMass() {
		t.Fatal("经 SetXxx 给出（含 0）的字段必须报告为已给出")
	}

	// 字面量直接构造（内置档/内核代码路径）沿用历史“非零即给出”口径。
	lit := Solution{MassConcentration: 5, MolarMass: 58.44}
	if lit.HasMolarity() || !lit.HasMassConcentration() || !lit.HasMolarMass() {
		t.Fatal("字面量构造的存在性应按非零推断，内置质量口径构造被误判")
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
