package membrane

import (
	"fmt"
	"math"

	"rocalc/internal/validation"
)

// IsFullRejection 判断盐截留率是否“贴近一”，按完全截留极限处理。
func IsFullRejection(r float64) bool {
	return r >= 1-FullRejectionEps
}

// PermeateMolarity 计算产水摩尔浓度。
//
// 完全截留（r=1）时产水不含盐：Cp = 0；
// 否则产水带盐，按截留率定义 Cp = Cf·(1−r)。
func PermeateMolarity(cf, r float64) (float64, error) {
	if err := validation.Rejection(r); err != nil {
		return 0, err
	}
	if err := validation.MolarConcentration(cf); err != nil {
		return 0, err
	}
	if IsFullRejection(r) {
		return 0, nil
	}
	return cf * (1 - r), nil
}

// BrineMolarity 计算浓水（盐水）摩尔浓度，单位 mol/L。
//
// 由整系统盐守恒 Cf·Qf = Cp·Qp + Cb·Qb，配合 Qb = Qf−Qp、Y = Qp/Qf 推导：
//
//	完全截留 r=1（Cp=0）：Cb = Cf / (1−Y)                 —— 极限式
//	部分截留 r<1（Cp=Cf(1−r)）：Cb = Cf·[1−(1−r)·Y]/(1−Y)
//
// 回收越高浓水越浓；绝不能写成 Cf·(1−Y)——那会让浓水随回收变稀、守恒崩塌。
func BrineMolarity(cf, y, r float64) (float64, error) {
	if err := validation.Recovery(y); err != nil {
		return 0, err
	}
	if err := validation.Rejection(r); err != nil {
		return 0, err
	}
	if err := validation.MolarConcentration(cf); err != nil {
		return 0, err
	}
	if IsFullRejection(r) {
		return cf / (1 - y), nil
	}
	return cf * (1 - (1-r)*y) / (1 - y), nil
}

// 衡算残差容差：相对 1e−9，浓度接近 0（纯水）时用绝对下限兜底。
const (
	balanceRelTol     = 1e-9
	balanceMolarFloor = 1e-12 // mol/h
	balanceMassFloor  = 1e-12 // g/h
)

// CheckBalance 校验一次核算结果是否满足整系统盐物料守恒：
//
//	进料盐量 = 产水盐量 + 浓水盐量
//
// 摩尔口径必校（mol/h）；工况给了摩尔质量时再校质量口径（g/h）。
// 任一口径不闭合都返回 CodeSaltBalanceViolation。
func CheckBalance(out *Outcome, spec FeedSpec) error {
	feedMol := out.FeedMolarity * spec.FeedFlow
	permMol := out.PermeateMolarity * out.PermeateFlow
	brineMol := out.BrineMolarity * out.BrineFlow
	residualMol := feedMol - permMol - brineMol

	tolMol := balanceRelTol * math.Max(math.Abs(feedMol), math.Max(math.Abs(permMol), math.Abs(brineMol)))
	if tolMol < balanceMolarFloor {
		tolMol = balanceMolarFloor
	}
	if math.Abs(residualMol) > tolMol {
		return validation.NewError(validation.CodeSaltBalanceViolation,
			fmt.Sprintf("摩尔盐量不守恒：进料 %.9g mol/h ≠ 产水 %.9g + 浓水 %.9g（残差 %.3g mol/h）",
				feedMol, permMol, brineMol, residualMol))
	}

	if spec.Feed.MolarMass > 0 {
		m := spec.Feed.MolarMass
		out.Salt.FeedSaltMassFlow = feedMol * m
		out.Salt.PermeateSaltMassFlow = permMol * m
		out.Salt.BrineSaltMassFlow = brineMol * m
		residualMass := out.Salt.FeedSaltMassFlow - out.Salt.PermeateSaltMassFlow - out.Salt.BrineSaltMassFlow
		out.Salt.Residual = residualMass

		tolMass := balanceRelTol * math.Max(math.Abs(out.Salt.FeedSaltMassFlow),
			math.Max(math.Abs(out.Salt.PermeateSaltMassFlow), math.Abs(out.Salt.BrineSaltMassFlow)))
		if tolMass < balanceMassFloor {
			tolMass = balanceMassFloor
		}
		if math.Abs(residualMass) > tolMass {
			return validation.NewError(validation.CodeSaltBalanceViolation,
				fmt.Sprintf("质量盐量不守恒：进料 %.9g g/h ≠ 产水 %.9g + 浓水 %.9g（残差 %.3g g/h）",
					out.Salt.FeedSaltMassFlow, out.Salt.PermeateSaltMassFlow,
					out.Salt.BrineSaltMassFlow, residualMass))
		}
	}
	return nil
}
