package cases

import (
	"errors"
	"math"
	"sync"
	"testing"

	"rocalc/internal/membrane"
)

func TestBuiltinBrackish_FewBarAndBalanced(t *testing.T) {
	r := NewDefaultRegistry()
	out, err := r.Evaluate(DefaultBrackishCase)
	if err != nil {
		t.Fatalf("内置苦咸水档必须可直接核算: %v", err)
	}
	// 手算：π = 2×(5/58.44)×0.08314×298.15 ≈ 4.24 bar
	if !(out.FeedOsmoticPressure > 3 && out.FeedOsmoticPressure < 5) {
		t.Fatalf("渗透压应在几个巴量级，实际 %.4f bar", out.FeedOsmoticPressure)
	}
	t.Logf("内置档：πf=%.4f bar, NDP=%.4f bar, Qp=%.2f L/h, Y=%.4f, Cb=%.5f mol/L (%.2f g/L)",
		out.FeedOsmoticPressure, out.NetDrivingPressure, out.PermeateFlow,
		out.Recovery, out.BrineMolarity, out.BrineMolarity*58.44)
	// 完全截留：产水零盐，进料盐全进浓水。
	if out.PermeateMolarity != 0 {
		t.Fatal("内置档完全截留，产水应为零盐")
	}
	resid := out.FeedMolarity*1000 - out.BrineMolarity*out.BrineFlow
	if math.Abs(resid) > 1e-9 {
		t.Fatalf("内置档盐量不守恒，残差 %g", resid)
	}
}

func TestRegistry_TwoCasesStayIndependent(t *testing.T) {
	// 同一进程两份不同工况档：结果各自独立、不能串到对方名下。
	r := NewDefaultRegistry()
	a := BuiltinBrackishSpec()
	a.Name = "case-a"
	b := BuiltinBrackishSpec()
	b.Name = "case-b"
	b.AppliedPressure = 15
	b.Area = 30
	if err := r.Register(a); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(b); err != nil {
		t.Fatal(err)
	}

	oa, err := r.Evaluate("case-a")
	if err != nil {
		t.Fatal(err)
	}
	ob, err := r.Evaluate("case-b")
	if err != nil {
		t.Fatal(err)
	}

	// 再取一次 a，结果必须与首次一致（计算 b 不能污染 a 的登记规格）。
	oa2, err := r.Evaluate("case-a")
	if err != nil {
		t.Fatal(err)
	}
	if oa.Recovery != oa2.Recovery || oa.NetDrivingPressure != oa2.NetDrivingPressure {
		t.Fatal("工况档 a 在核算 b 后结果发生漂移，存在串档")
	}
	if oa.Recovery == ob.Recovery {
		t.Fatal("两份不同工况档结果不应相同")
	}

	// 取回复本，直接改副本，登记表内的原件不受影响。
	gotA, _ := r.Get("case-a")
	gotA.AppliedPressure = 999
	oa3, err := r.Evaluate("case-a")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(oa3.NetDrivingPressure-oa.NetDrivingPressure) > 1e-12 {
		t.Fatal("修改取出的副本污染了登记表内的工况档")
	}
}

func TestRegistry_RegisterDuplicateAndMissing(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(BuiltinBrackishSpec()); err != nil {
		t.Fatal(err)
	}
	err := r.Register(BuiltinBrackishSpec())
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("重复登记应返回 ErrAlreadyExists，实际 %v", err)
	}
	if _, err := r.Get("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("取不到档应 ErrNotFound，实际 %v", err)
	}
	if _, err := r.Evaluate("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("核算不存在的档应 ErrNotFound，实际 %v", err)
	}
}

func TestRegistry_UpsertReplaces(t *testing.T) {
	r := NewDefaultRegistry()
	spec := BuiltinBrackishSpec()
	spec.AppliedPressure = 14
	r.Put(spec)
	out, err := r.Evaluate(DefaultBrackishCase)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(out.NetDrivingPressure-(14-out.FeedOsmoticPressure)) > 1e-9 {
		t.Fatal("Put 应整体替换工况档")
	}
}

func TestRegistry_ConcurrentEvaluation(t *testing.T) {
	r := NewDefaultRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := r.Evaluate(DefaultBrackishCase)
			if err != nil {
				t.Errorf("并发核算出错: %v", err)
				return
			}
			if !(out.Recovery > 0 && out.Recovery < 1) {
				t.Errorf("并发下回收率异常: %g", out.Recovery)
			}
		}()
	}
	wg.Wait()
}

func TestBuiltinSpec_NameAndComposition(t *testing.T) {
	spec := BuiltinBrackishSpec()
	if spec.Name != DefaultBrackishCase {
		t.Fatal("内置档名称不符")
	}
	if spec.Feed.MassConcentration <= 0 || spec.Feed.MolarMass <= 0 {
		t.Fatal("内置档应以质量浓度口径登记")
	}
	// 确保内核认这份结构。
	if _, err := membrane.Evaluate(spec.FeedSpec); err != nil {
		t.Fatal(err)
	}
}
