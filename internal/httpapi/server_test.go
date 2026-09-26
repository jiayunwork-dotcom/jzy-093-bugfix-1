package httpapi

import (
	"bytes"
	"encoding/json"
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
