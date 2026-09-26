package httpapi

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rocalc/internal/cases"
)

func newTestServer() (*Server, *httptest.Server) {
	srv := NewServer()
	return srv, httptest.NewServer(srv.Handler())
}

func doJSON(t *testing.T, ts *httptest.Server, method, path string, body string) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, ts.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("响应不是 JSON: %v", err)
	}
	return resp.StatusCode, m
}

func TestHealthAndConstants(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	if code, body := doJSON(t, ts, http.MethodGet, "/healthz", ""); code != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("health 异常: %d %v", code, body)
	}
	code, body := doJSON(t, ts, http.MethodGet, "/v1/constants", "")
	if code != http.StatusOK {
		t.Fatalf("constants 状态码 %d", code)
	}
	r, _ := body["gas_constant_r_bar_l_mol_k"].(float64)
	if r < 0.083 || r > 0.0832 {
		t.Fatalf("R 取值异常: %v", body["gas_constant_r_bar_l_mol_k"])
	}
}

func TestEvaluateBuiltinCaseOverHTTP(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	code, list := doJSON(t, ts, http.MethodGet, "/v1/cases", "")
	if code != http.StatusOK {
		t.Fatalf("列工况档失败: %v", list)
	}

	code, body := doJSON(t, ts, http.MethodPost,
		"/v1/cases/"+cases.DefaultBrackishCase+"/evaluate", "")
	if code != http.StatusOK {
		t.Fatalf("内置档核算失败: %d %v", code, body)
	}
	pi, _ := body["feed_osmotic_pressure_bar"].(float64)
	if pi < 3 || pi > 5 {
		t.Fatalf("渗透压应几个巴，实际 %v", pi)
	}
	y, _ := body["recovery"].(float64)
	if y <= 0 || y >= 1 {
		t.Fatalf("回收率越界: %v", y)
	}
	permMolar, _ := body["permeate_molarity_mol_per_l"].(float64)
	if permMolar != 0 {
		t.Fatal("内置档完全截留，产水应为零盐")
	}
	units, _ := body["units"].(map[string]any)
	if units["pressure"] != "bar" {
		t.Fatal("响应应明确压力单位为 bar")
	}
}

func TestAdhocEvaluate_BelowOsmoticPressureRejected(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	// 压差 3 bar 低于渗透压（约 4.24 bar）：必须 400 + non_positive_ndp，
	// 且响应里不得出现正产水流量字段（根本不返回 outcome）。
	body := `{
	  "feed": {"mass_concentration_g_per_l": 5, "molar_mass_g_per_mol": 58.44,
	           "temperature_k": 298.15, "vanth_hoff_factor": 2},
	  "applied_pressure_bar": 3, "feed_flow_lh": 1000,
	  "permeability_lmh_per_bar": 1000, "area_m2": 1000,
	  "salt_rejection": 1, "polarization_factor": 1
	}`
	code, resp := doJSON(t, ts, http.MethodPost, "/v1/evaluate", body)
	if code != http.StatusBadRequest {
		t.Fatalf("应返回 400，实际 %d: %v", code, resp)
	}
	if resp["code"] != "non_positive_ndp" {
		t.Fatalf("错误码应为 non_positive_ndp，实际 %v", resp["code"])
	}
	if _, present := resp["permeate_flow_lh"]; present {
		t.Fatal("非法工况不得报告产水流量")
	}
}

func TestAdhocEvaluate_PartialRejectionBalanced(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	body := `{
	  "feed": {"mass_concentration_g_per_l": 5, "molar_mass_g_per_mol": 58.44,
	           "temperature_k": 298.15, "vanth_hoff_factor": 2},
	  "applied_pressure_bar": 12, "feed_flow_lh": 1000,
	  "permeability_lmh_per_bar": 1.8, "area_m2": 36,
	  "salt_rejection": 0.98
	}`
	code, resp := doJSON(t, ts, http.MethodPost, "/v1/evaluate", body)
	if code != http.StatusOK {
		t.Fatalf("部分截留合法工况应 200，实际 %d %v", code, resp)
	}
	salt, _ := resp["salt_balance"].(map[string]any)
	residual, _ := salt["residual_gh"].(float64)
	if residual > 1e-6 || residual < -1e-6 {
		t.Fatalf("质量盐量残差应近零，实际 %v", residual)
	}
	feedSalt, _ := salt["feed_salt_mass_flow_gh"].(float64)
	permSalt, _ := salt["permeate_salt_mass_flow_gh"].(float64)
	brineSalt, _ := salt["brine_salt_mass_flow_gh"].(float64)
	if feedSalt < permSalt+brineSalt-1e-6 || feedSalt > permSalt+brineSalt+1e-6 {
		t.Fatalf("HTTP 结果盐量不守恒: %v vs %v+%v", feedSalt, permSalt, brineSalt)
	}
}

func TestCreateThenEvaluateNamedCase(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	body := `{
	  "name": "bitter-well-1",
	  "feed": {"molarity_mol_per_l": 0.09, "temperature_k": 303.15, "vanth_hoff_factor": 2},
	  "applied_pressure_bar": 10, "feed_flow_lh": 800,
	  "permeability_lmh_per_bar": 2, "area_m2": 30,
	  "salt_rejection": 1
	}`
	if code, resp := doJSON(t, ts, http.MethodPost, "/v1/cases", body); code != http.StatusCreated {
		t.Fatalf("登记工况档失败: %d %v", code, resp)
	}
	// 重复登记 409
	if code, _ := doJSON(t, ts, http.MethodPost, "/v1/cases", body); code != http.StatusConflict {
		t.Fatalf("重名应 409，实际 %d", code)
	}
	code, resp := doJSON(t, ts, http.MethodPost, "/v1/cases/bitter-well-1/evaluate", "")
	if code != http.StatusOK {
		t.Fatalf("点名核算失败: %d %v", code, resp)
	}
	if y, _ := resp["recovery"].(float64); y <= 0 || y >= 1 {
		t.Fatalf("回收率越界: %v", y)
	}
}

func TestUnknownCaseAndBadPayload(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	code, resp := doJSON(t, ts, http.MethodPost, "/v1/cases/ghost/evaluate", "")
	if code != http.StatusNotFound || resp["code"] != "case_not_found" {
		t.Fatalf("未知档应 404 case_not_found，实际 %d %v", code, resp)
	}

	// 温度非正 → 400 invalid_temperature
	bad := `{"feed":{"molarity_mol_per_l":0.1,"temperature_k":-1,"vanth_hoff_factor":2},
	         "applied_pressure_bar":12,"feed_flow_lh":1000,
	         "permeability_lmh_per_bar":1.8,"area_m2":36,"salt_rejection":1}`
	code, resp = doJSON(t, ts, http.MethodPost, "/v1/evaluate", bad)
	if code != http.StatusBadRequest || resp["code"] != "invalid_temperature" {
		t.Fatalf("非法温度应 400 invalid_temperature，实际 %d %v", code, resp)
	}

	// 坏 JSON → 400 malformed_json
	code, resp = doJSON(t, ts, http.MethodPost, "/v1/evaluate", "{not json")
	if code != http.StatusBadRequest || resp["code"] != "malformed_json" {
		t.Fatalf("坏 JSON 应 400 malformed_json，实际 %d %v", code, resp)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/v1/cases", strings.NewReader("{}"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("PATCH /v1/cases 应 405，实际 %d", resp.StatusCode)
	}
}

func TestPutUpsertAndDelete(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	body := `{
	  "feed": {"molarity_mol_per_l": 0.1, "temperature_k": 298.15, "vanth_hoff_factor": 2},
	  "applied_pressure_bar": 10, "feed_flow_lh": 500,
	  "permeability_lmh_per_bar": 1.5, "area_m2": 20, "salt_rejection": 1
	}`
	if code, resp := doJSON(t, ts, http.MethodPut, "/v1/cases/editable", body); code != http.StatusOK {
		t.Fatalf("PUT 新建失败: %d %v", code, resp)
	}
	if code, _ := doJSON(t, ts, http.MethodGet, "/v1/cases/editable", ""); code != http.StatusOK {
		t.Fatalf("GET 新档失败: %d", code)
	}
	if code, _ := doJSON(t, ts, http.MethodDelete, "/v1/cases/editable", ""); code != http.StatusOK {
		t.Fatalf("DELETE 失败: %d", code)
	}
	if code, _ := doJSON(t, ts, http.MethodGet, "/v1/cases/editable", ""); code != http.StatusNotFound {
		t.Fatalf("删除后应 404，实际 %d", code)
	}
}

// 三种常用合法写法（只给质量浓度+摩尔质量、只给摩尔浓度、两者自洽同时给）
// 算出来的数必须与改动前一位不差：三者本应描述同一份 5 g/L NaCl 进料。
func TestEvaluate_LegalConcentrationForms_UnchangedNumbers(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	spec := func(feedFields string) string {
		return `{"feed":{` + feedFields + `,"temperature_k":298.15,"vanth_hoff_factor":2},
		  "applied_pressure_bar":12,"feed_flow_lh":1000,
		  "permeability_lmh_per_bar":1.8,"area_m2":36,"salt_rejection":1}`
	}
	massOnly := spec(`"mass_concentration_g_per_l":5,"molar_mass_g_per_mol":58.44`)
	molarOnly := spec(`"molarity_mol_per_l":0.08555783709787818`)
	both := spec(`"molarity_mol_per_l":0.08555783709787818,
	               "mass_concentration_g_per_l":5,"molar_mass_g_per_mol":58.44`)

	var ref map[string]any
	for i, body := range []string{massOnly, molarOnly, both} {
		code, resp := doJSON(t, ts, http.MethodPost, "/v1/evaluate", body)
		if code != http.StatusOK {
			t.Fatalf("写法 %d 应 200，实际 %d %v", i, code, resp)
		}
		if i == 0 {
			ref = resp
			continue
		}
		for _, field := range []string{
			"feed_molarity_mol_per_l", "feed_osmotic_pressure_bar",
			"net_driving_pressure_bar", "permeate_flow_lh", "recovery",
			"brine_molarity_mol_per_l",
		} {
			if resp[field] != ref[field] {
				t.Fatalf("写法 %d 的 %s=%v 与只给质量口径的 %v 不一致",
					i, field, resp[field], ref[field])
			}
		}
	}

	// 内置苦咸水档点名核算仍是约 4.24 bar 渗透压。
	code, builtin := doJSON(t, ts, http.MethodPost,
		"/v1/cases/"+cases.DefaultBrackishCase+"/evaluate", "")
	if code != http.StatusOK {
		t.Fatalf("内置档核算失败: %d %v", code, builtin)
	}
	pi, _ := builtin["feed_osmotic_pressure_bar"].(float64)
	if math.Abs(pi-4.2416) > 0.01 {
		t.Fatalf("内置档渗透压漂移：%.6f bar", pi)
	}
}

// 两种纯水写法（只带摩尔浓度 0；摩尔 0、质量 0、摩尔质量为正）照旧按 0 mol/L
// 正常出结果，渗透压为 0，且显式给的 0 原样回显。
func TestEvaluate_PureWaterForms_StillValid(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	spec := func(feedFields string) string {
		return `{"feed":{` + feedFields + `,"temperature_k":298.15,"vanth_hoff_factor":2},
		  "applied_pressure_bar":12,"feed_flow_lh":1000,
		  "permeability_lmh_per_bar":1.8,"area_m2":36,"salt_rejection":1}`
	}
	for i, body := range []string{
		spec(`"molarity_mol_per_l":0`),
		spec(`"molarity_mol_per_l":0,"mass_concentration_g_per_l":0,"molar_mass_g_per_mol":58.44`),
	} {
		code, resp := doJSON(t, ts, http.MethodPost, "/v1/evaluate", body)
		if code != http.StatusOK {
			t.Fatalf("纯水写法 %d 应 200，实际 %d %v", i, code, resp)
		}
		if resp["feed_molarity_mol_per_l"] != 0.0 || resp["feed_osmotic_pressure_bar"] != 0.0 {
			t.Fatalf("纯水写法 %d 应按 0 mol/L、π=0 出结果，实际 %v", i, resp)
		}
	}
}

// 组合一：显式摩尔浓度 0 撞上非零质量浓度（且摩尔质量为正），临时核算必须和
// 给 0.05 一样回 400 inconsistent_concentration，原因里带两边的数值。
func TestAdhocEvaluate_ExplicitZeroMolarityConflictsWithMass(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	body := `{
	  "feed": {"molarity_mol_per_l": 0, "mass_concentration_g_per_l": 5,
	           "molar_mass_g_per_mol": 58.44,
	           "temperature_k": 298.15, "vanth_hoff_factor": 2},
	  "applied_pressure_bar": 12, "feed_flow_lh": 1000,
	  "permeability_lmh_per_bar": 1.8, "area_m2": 36, "salt_rejection": 1
	}`
	code, resp := doJSON(t, ts, http.MethodPost, "/v1/evaluate", body)
	if code != http.StatusBadRequest {
		t.Fatalf("显式 0 与非零质量浓度冲突应 400，实际 %d %v", code, resp)
	}
	if resp["code"] != "inconsistent_concentration" {
		t.Fatalf("错误码应为 inconsistent_concentration，实际 %v", resp["code"])
	}
	reason, _ := resp["reason"].(string)
	for _, want := range []string{"0 mol/L", "0.0855", "5 g/L", "58.44 g/mol"} {
		if !strings.Contains(reason, want) {
			t.Fatalf("拒绝原因应带上两边数值（缺 %q），实际：%s", want, reason)
		}
	}
	if _, present := resp["feed_osmotic_pressure_bar"]; present {
		t.Fatal("非法工况不得返回核算结果")
	}

	// 同一份内容走登记档点名核算，必须给出同一个判定（不能一个拒、一个算出结果）。
	register := strings.Replace(body, `"applied_pressure_bar"`, `"name":"explicit-zero",`+`"applied_pressure_bar"`, 1)
	if code, reg := doJSON(t, ts, http.MethodPost, "/v1/cases", register); code != http.StatusCreated {
		t.Fatalf("登记本身按现状允许保存（核算时再判），实际 %d %v", code, reg)
	}
	code, named := doJSON(t, ts, http.MethodPost, "/v1/cases/explicit-zero/evaluate", "")
	if code != http.StatusBadRequest || named["code"] != "inconsistent_concentration" {
		t.Fatalf("点名核算应与临时核算同判 400 inconsistent_concentration，实际 %d %v", code, named)
	}
}

// 组合二：质量浓度显式出现（哪怕是 0）却缺摩尔质量，和写成 5 一样拦下；
// 反过来摩尔浓度 0.1 + 质量浓度 0（不给摩尔质量）也不得照 0.1 放行。
func TestAdhocEvaluate_ExplicitZeroMassRequiresMolarMass(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	spec := func(feedFields string) string {
		return `{"feed":{` + feedFields + `,"temperature_k":298.15,"vanth_hoff_factor":2},
		  "applied_pressure_bar":12,"feed_flow_lh":1000,
		  "permeability_lmh_per_bar":1.8,"area_m2":36,"salt_rejection":1}`
	}
	for i, body := range []string{
		spec(`"mass_concentration_g_per_l":0`),
		spec(`"molarity_mol_per_l":0.1,"mass_concentration_g_per_l":0`),
	} {
		code, resp := doJSON(t, ts, http.MethodPost, "/v1/evaluate", body)
		if code != http.StatusBadRequest {
			t.Fatalf("组合 %d 应 400，实际 %d %v", i, code, resp)
		}
		if resp["code"] != "non_positive_parameter" {
			t.Fatalf("组合 %d 缺摩尔质量应报 non_positive_parameter（与写 5 同码），实际 %v",
				i, resp["code"])
		}
	}

	// 对照：质量浓度写 5、不给摩尔质量，历来就是这个码。
	code, resp := doJSON(t, ts, http.MethodPost, "/v1/evaluate",
		spec(`"mass_concentration_g_per_l":5`))
	if code != http.StatusBadRequest || resp["code"] != "non_positive_parameter" {
		t.Fatalf("质量浓度 5 缺摩尔质量应对照为 400 non_positive_parameter，实际 %d %v", code, resp)
	}
}

// 组合三：显式给 0 的档登记成功后 GET 回来，字段必须与提交一致——0 原样出现，
// 不能从返回里蒸发；点名核算的判定与临时核算一致。
func TestRegisteredCase_PreservesExplicitZeroOnReadback(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	body := `{
	  "name": "zero-roundtrip",
	  "feed": {"molarity_mol_per_l": 0, "mass_concentration_g_per_l": 5,
	           "molar_mass_g_per_mol": 58.44,
	           "temperature_k": 298.15, "vanth_hoff_factor": 2},
	  "applied_pressure_bar": 12, "feed_flow_lh": 1000,
	  "permeability_lmh_per_bar": 1.8, "area_m2": 36, "salt_rejection": 1
	}`
	if code, resp := doJSON(t, ts, http.MethodPost, "/v1/cases", body); code != http.StatusCreated {
		t.Fatalf("登记失败: %d %v", code, resp)
	}
	code, got := doJSON(t, ts, http.MethodGet, "/v1/cases/zero-roundtrip", "")
	if code != http.StatusOK {
		t.Fatalf("GET 失败: %d %v", code, got)
	}
	feed, ok := got["feed"].(map[string]any)
	if !ok {
		t.Fatalf("返回缺 feed: %v", got)
	}
	molarity, present := feed["molarity_mol_per_l"]
	if !present {
		t.Fatal("显式给的摩尔浓度 0 必须原样出现在 GET 返回里，实际字段消失")
	}
	if molarity != 0.0 {
		t.Fatalf("回显的摩尔浓度应为 0，实际 %v", molarity)
	}
	if feed["mass_concentration_g_per_l"] != 5.0 || feed["molar_mass_g_per_mol"] != 58.44 {
		t.Fatalf("其余浓度字段也应与提交一致: %v", feed)
	}

	code, evaluated := doJSON(t, ts, http.MethodPost, "/v1/cases/zero-roundtrip/evaluate", "")
	if code != http.StatusBadRequest || evaluated["code"] != "inconsistent_concentration" {
		t.Fatalf("登记档点名核算必须与临时核算同判，实际 %d %v", code, evaluated)
	}
}
