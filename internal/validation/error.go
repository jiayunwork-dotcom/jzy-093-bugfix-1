// Package validation 集中放置膜过程核算的输入合法性检查与领域错误定义。
// 其它计算包（osmotic、membrane）只依赖这里的检查函数与错误类型，
// 不自行拼装错误，保证非法输入的拒绝口径统一。
package validation

// DomainError 是一类“工况本身不合法/不物理”的错误，携带稳定的错误码，
// 方便 HTTP 层映射状态码、调用方按码分支处理。
type DomainError struct {
	Code   string
	Reason string
}

func (e *DomainError) Error() string {
	if e.Reason == "" {
		return e.Code
	}
	return e.Code + ": " + e.Reason
}

// NewError 构造一个领域错误。
func NewError(code, reason string) *DomainError {
	return &DomainError{Code: code, Reason: reason}
}

// 稳定错误码：对外契约的一部分，不要随意改名。
const (
	// CodeNegativeConcentration 浓度（摩尔/质量）为负。
	CodeNegativeConcentration = "negative_concentration"
	// CodeInvalidTemperature 温度不为正（van't Hoff 要求开尔文温度 T>0）。
	CodeInvalidTemperature = "invalid_temperature"
	// CodeInvalidVanTHoff 范特霍夫因子不为正。
	CodeInvalidVanTHoff = "invalid_vanthoff_factor"
	// CodeNonPositiveParameter 通用的“必须为正”的参数不合法。
	CodeNonPositiveParameter = "non_positive_parameter"
	// CodeInconsistentConcentration 直接给的摩尔浓度与质量浓度/摩尔质量换算值不自洽。
	CodeInconsistentConcentration = "inconsistent_concentration"
	// CodeInvalidPolarization 浓差极化因子不合法（必须 >= 1）。
	CodeInvalidPolarization = "invalid_polarization_factor"
	// CodeInvalidRejection 盐截留率不在 (0,1]。
	CodeInvalidRejection = "invalid_rejection"
	// CodeInvalidRecovery 回收率不在开区间 (0,1)。
	CodeInvalidRecovery = "invalid_recovery"
	// CodeNonPositiveNDP 净推动力不为正，却出现/要求了正产水通量。
	CodeNonPositiveNDP = "non_positive_ndp"
	// CodeSaltBalanceViolation 进料盐量不等于产水盐量加浓水盐量。
	CodeSaltBalanceViolation = "salt_balance_violation"
)
