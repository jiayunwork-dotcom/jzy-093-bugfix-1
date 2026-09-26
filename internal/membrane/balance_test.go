package membrane

import (
	"math"
	"testing"

	"rocalc/internal/osmotic"
)

// 衡算独立测试：不经过 Evaluate 的封装，直接盯“进料 = 产水 + 浓水”。
func TestCheckBalance_MolarClosure(t *testing.T) {
	spec := baseBrackish()
	spec.Rejection = 0.97
	out := &Outcome{
		FeedMolarity: 5.0 / 58.44,
		PermeateFlow: 0,
		BrineFlow:    0,
	}
	cf := out.FeedMolarity
	qf := spec.FeedFlow
	// 手构一组满足守恒的数值：Y=0.45，r=0.97。
	y := 0.45
	out.PermeateFlow = y * qf
	out.BrineFlow = qf - out.PermeateFlow
	out.PermeateMolarity = cf * (1 - spec.Rejection)
	out.BrineMolarity = cf * (1 - (1-spec.Rejection)*y) / (1 - y)

	if err := CheckBalance(out, spec); err != nil {
		t.Fatalf("守恒闭合的衡算不应报错: %v", err)
	}

	// 人为破坏浓水浓度（即关键坑 Cf·(1−Y)），必须被守恒校验抓住。
	out.BrineMolarity = cf * (1 - y)
	if err := CheckBalance(out, spec); err == nil {
		t.Fatal("Cb 被错写成 Cf·(1−Y) 时必须报盐量不守恒")
	}
}

func TestPermeateMolarity_FullRejectionIsZero(t *testing.T) {
	cp, err := PermeateMolarity(0.1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if cp != 0 {
		t.Fatalf("r=1 时产水含盐必须为零，实际 %g", cp)
	}
}

func TestBrineMolarity_FullRejectionAllSaltToBrine(t *testing.T) {
	// r=1、Y=0.5：浓水浓度恰为进料两倍，盐全在浓水里。
	cb, err := BrineMolarity(0.1, 0.5, 1)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(cb-0.2) > 1e-12 {
		t.Fatalf("Cb 应为 0.2 mol/L，实际 %g", cb)
	}
}

func TestOsmoticAndPressureUnitsStayInBar(t *testing.T) {
	// 防量纲偷换：π 与 NDP 都在 bar 量纲下直接相减，
	// 以 0.1 mol/L NaCl、298.15 K 手算 π≈4.958 bar，施加 6 bar 时 NDP≈1.04 bar。
	pi, err := osmotic.Pressure(osmotic.Solution{Molarity: 0.1, Temperature: 298.15, VanTHoff: 2})
	if err != nil {
		t.Fatal(err)
	}
	ndp := NetDrivingPressure(6, pi, 0, 1)
	if math.Abs(ndp-(6-pi)) > 1e-12 || ndp <= 0 {
		t.Fatalf("同量纲相减异常：π=%.4f, NDP=%.4f", pi, ndp)
	}
}
