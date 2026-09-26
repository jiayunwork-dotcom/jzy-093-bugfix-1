package membrane

import (
	"errors"
	"math"
	"testing"

	"rocalc/internal/osmotic"
	"rocalc/internal/validation"
)

// baseBrackish 返回一份合法的内置苦咸水工况（完全截留、无极化）。
func baseBrackish() FeedSpec {
	return FeedSpec{
		Feed: osmotic.Solution{
			MassConcentration: 5.0,
			MolarMass:         58.44,
			Temperature:       298.15,
			VanTHoff:          2,
		},
		AppliedPressure: 12,
		FeedFlow:        1000,
		Permeability:    1.8,
		Area:            36,
		Rejection:       1,
		Polarization:    1,
	}
}

func codeOf(err error) string {
	var de *validation.DomainError
	if errors.As(err, &de) {
		return de.Code
	}
	return ""
}

func TestEvaluate_FullRejection_HandCalc(t *testing.T) {
	out, err := Evaluate(baseBrackish())
	if err != nil {
		t.Fatalf("内置苦咸水档应当合法: %v", err)
	}
	if out.PermeateMolarity != 0 {
		t.Fatalf("完全截留时产水含盐必须为 0，实际 %g mol/L", out.PermeateMolarity)
	}
	if !(out.Recovery > 0 && out.Recovery < 1) {
		t.Fatalf("回收率必须落在 (0,1)，实际 %g", out.Recovery)
	}
	// 完全截留极限：Cb = Cf/(1−Y)
	wantCb := out.FeedMolarity / (1 - out.Recovery)
	if math.Abs(out.BrineMolarity-wantCb) > 1e-10 {
		t.Fatalf("浓水浓度 %.9g 与极限式 Cf/(1−Y)=%.9g 不符", out.BrineMolarity, wantCb)
	}
	// 关键坑防守：绝不能是 Cf·(1−Y)（回收越高反而越稀）
	wrong := out.FeedMolarity * (1 - out.Recovery)
	if math.Abs(out.BrineMolarity-wrong) < 1e-9 {
		t.Fatal("浓水浓度退化成了 Cf·(1−Y)，盐守恒必然崩塌")
	}
	if out.BrineMolarity <= out.FeedMolarity {
		t.Fatal("有回收时浓水必须比进料更浓")
	}
	// 摩尔盐量守恒（完全截留：进料盐全进浓水）
	resid := out.FeedMolarity*1000 - out.BrineMolarity*out.BrineFlow
	if math.Abs(resid) > 1e-9 {
		t.Fatalf("盐量守恒残差 %g mol/h 超容差", resid)
	}
}

func TestEvaluate_BelowOsmoticPressure_NoPositiveFlux(t *testing.T) {
	// 重点防守：工作压差（3 bar）低于进料渗透压（约 4.24 bar），
	// 哪怕膜面积、渗透系数给得再大，也绝不能冒出正通量。
	spec := baseBrackish()
	spec.AppliedPressure = 3
	spec.Area = 1e6
	spec.Permeability = 1e6

	out, err := Evaluate(spec)
	if err == nil {
		t.Fatalf("NDP 为负必须拒绝，却返回了结果 Qp=%g", out.PermeateFlow)
	}
	if codeOf(err) != validation.CodeNonPositiveNDP {
		t.Fatalf("期望错误码 %s，实际 %v", validation.CodeNonPositiveNDP, err)
	}
	if out != nil && out.PermeateFlow > 0 {
		t.Fatalf("非法工况下不得报告正产水通量，Qp=%g", out.PermeateFlow)
	}
}

func TestNDP_EqualsOsmoticPressure_ZeroFlux(t *testing.T) {
	// 交叉关系：Δp 恰好等于进料渗透压、且无极化时，NDP=0、Qp=0。
	spec := baseBrackish()
	piF, err := osmotic.Pressure(spec.Feed)
	if err != nil {
		t.Fatal(err)
	}
	ndp := NetDrivingPressure(piF, piF, 0, 1)
	if ndp != 0 {
		t.Fatalf("Δp=πf 且无极化时 NDP 必须精确为 0，实际 %.12g", ndp)
	}
	qp := PermeateFlow(spec.Area, spec.Permeability, ndp)
	if qp != 0 {
		t.Fatalf("NDP=0 时产水流量必须为 0，实际 %g", qp)
	}
}

func TestEvaluate_PartialRejection_SaltBalance(t *testing.T) {
	// r=0.98：产水带盐，全系统必须盐量守恒。
	spec := baseBrackish()
	spec.Rejection = 0.98
	out, err := Evaluate(spec)
	if err != nil {
		t.Fatalf("部分截留合法工况不应报错: %v", err)
	}
	if out.PermeateMolarity <= 0 {
		t.Fatal("部分截留时产水必须带盐")
	}
	wantCp := out.FeedMolarity * (1 - 0.98)
	if math.Abs(out.PermeateMolarity-wantCp) > 1e-12 {
		t.Fatalf("产水浓度 %g 与 Cf(1−r)=%g 不符", out.PermeateMolarity, wantCp)
	}
	wantCb := out.FeedMolarity * (1 - (1-0.98)*out.Recovery) / (1 - out.Recovery)
	if math.Abs(out.BrineMolarity-wantCb) > 1e-9 {
		t.Fatalf("浓水浓度 %.12g 与衡算式 %.12g 不符", out.BrineMolarity, wantCb)
	}
	// 守恒由 Evaluate 内部 CheckBalance 把关；这里再独立算一遍摩尔口径。
	feed := out.FeedMolarity * spec.FeedFlow
	perm := out.PermeateMolarity * out.PermeateFlow
	brine := out.BrineMolarity * out.BrineFlow
	if math.Abs(feed-perm-brine) > 1e-9 {
		t.Fatalf("进料盐 %.9g ≠ 产水 %.9g + 浓水 %.9g", feed, perm, brine)
	}
	// 质量口径（g/h）：进料 5 g/L × 1000 L/h = 5000 g/h
	feedMass := spec.Feed.MassConcentration * spec.FeedFlow
	permMass := out.Salt.PermeateSaltMassFlow
	brineMass := out.Salt.BrineSaltMassFlow
	if math.Abs(feedMass-(permMass+brineMass)) > 1e-6 {
		t.Fatalf("质量盐量不守恒：%g ≠ %g + %g", feedMass, permMass, brineMass)
	}
}

func TestBrineMolarity_RisesWithRecovery(t *testing.T) {
	const cf, r = 0.1, 1.0
	prev := 0.0
	for _, y := range []float64{0.1, 0.3, 0.5, 0.7, 0.9} {
		cb, err := BrineMolarity(cf, y, r)
		if err != nil {
			t.Fatal(err)
		}
		if cb <= prev {
			t.Fatalf("回收率上升浓水浓度必须单调上升：Y=%g 时 Cb=%g，前值 %g", y, cb, prev)
		}
		prev = cb
	}
}

func TestBrineMolarity_RejectsBadRecovery(t *testing.T) {
	for _, y := range []float64{0, -0.1, 1, 1.2} {
		if _, err := BrineMolarity(0.1, y, 1); codeOf(err) != validation.CodeInvalidRecovery {
			t.Fatalf("Y=%g 应报 invalid_recovery，实际 %v", y, err)
		}
	}
}

func TestNDP_FallsWhenTemperatureRises(t *testing.T) {
	// 只提温度：π 升高，同一压差下 NDP 下降（完全截留时 πp=0）。
	cold := baseBrackish()
	hot := cold
	hot.Feed.Temperature = 318.15 // +20 K
	piCold, _ := osmotic.Pressure(cold.Feed)
	piHot, _ := osmotic.Pressure(hot.Feed)
	ndpCold := NetDrivingPressure(cold.AppliedPressure, piCold, 0, 1)
	ndpHot := NetDrivingPressure(hot.AppliedPressure, piHot, 0, 1)
	if !(piHot > piCold && ndpHot < ndpCold) {
		t.Fatalf("升温应使 π 升、NDP 降：π %.3f→%.3f，NDP %.3f→%.3f",
			piCold, piHot, ndpCold, ndpHot)
	}
}

func TestEvaluate_RecoveryAtLeastOneRejected(t *testing.T) {
	// 压差大到通量超过进料量：Y≥1 必须拒绝，且不得报正浓水。
	spec := baseBrackish()
	spec.AppliedPressure = 120
	out, err := Evaluate(spec)
	if codeOf(err) != validation.CodeInvalidRecovery {
		t.Fatalf("期望 invalid_recovery，实际 out=%v err=%v", out, err)
	}
}

func TestEvaluate_InvalidSpecs(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*FeedSpec)
		code   string
	}{
		{"温度非正", func(s *FeedSpec) { s.Feed.Temperature = 0 }, validation.CodeInvalidTemperature},
		{"因子非正", func(s *FeedSpec) { s.Feed.VanTHoff = 0 }, validation.CodeInvalidVanTHoff},
		{"零压差", func(s *FeedSpec) { s.AppliedPressure = 0 }, validation.CodeNonPositiveParameter},
		{"零进料流量", func(s *FeedSpec) { s.FeedFlow = 0 }, validation.CodeNonPositiveParameter},
		{"零膜面积", func(s *FeedSpec) { s.Area = 0 }, validation.CodeNonPositiveParameter},
		{"零渗透系数", func(s *FeedSpec) { s.Permeability = 0 }, validation.CodeNonPositiveParameter},
		{"极化因子小于一", func(s *FeedSpec) { s.Polarization = 0.9 }, validation.CodeInvalidPolarization},
		{"截留率大于一", func(s *FeedSpec) { s.Rejection = 1.2 }, validation.CodeInvalidRejection},
		{"截留率为零", func(s *FeedSpec) { s.Rejection = 0 }, validation.CodeInvalidRejection},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := baseBrackish()
			tc.mutate(&spec)
			_, err := Evaluate(spec)
			if codeOf(err) != tc.code {
				t.Fatalf("期望错误码 %s，实际 %v", tc.code, err)
			}
		})
	}
}

func TestEvaluate_PolarizationRaisesOsmoticPressure(t *testing.T) {
	plain := baseBrackish()
	pol := plain
	pol.Polarization = 1.2
	o1, err := Evaluate(plain)
	if err != nil {
		t.Fatal(err)
	}
	// 极化后 NDP 下降，同样面积/系数下回收更低。
	spec := pol
	// 12 bar 仍高于 1.2×πf≈5.1 bar，合法。
	o2, err := Evaluate(spec)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(o2.WallOsmoticPressure-1.2*o1.FeedOsmoticPressure) > 1e-9 {
		t.Fatal("膜面渗透压应为 β·πf")
	}
	if !(o2.NetDrivingPressure < o1.NetDrivingPressure && o2.Recovery < o1.Recovery) {
		t.Fatal("极化增强应降低 NDP 与回收率")
	}
}
