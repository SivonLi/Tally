//go:build integration

package pipeline_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"tally/internal/calc"
	"tally/internal/excel"
	"tally/internal/importer"
	"tally/internal/pipeline"
	"tally/internal/store"
)

func resolveHistoryDir(t *testing.T) string {
	t.Helper()
	candidates := []string{
		os.Getenv("TALLY_PRIVATE_DATA_DIR"),
		filepath.Join("..", "..", "private-data", "history"),
		"/Users/sivon/Developer/Github/Tally/private-data/history",
	}
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	t.Skip("private-data history directory not found; skipping integration test")
	return ""
}

// findMainWorkbook 在指定目录下挑选最主要的量化计算工作簿（忽略临时文件与副本，优先工作表数量最多者）。
func findMainWorkbook(t *testing.T, dir string, includeKeywords, excludeKeywords []string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	bestPath := ""
	bestSheets := -1
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "~$") || strings.HasPrefix(name, ".") || !strings.HasSuffix(strings.ToLower(name), ".xlsx") {
			continue
		}
		if strings.Contains(name, "副本") || strings.Contains(name, "备份") || strings.Contains(name, "新建") {
			continue
		}
		matched := len(includeKeywords) == 0
		for _, kw := range includeKeywords {
			if strings.Contains(name, kw) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		excluded := false
		for _, kw := range excludeKeywords {
			if strings.Contains(name, kw) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		p := filepath.Join(dir, name)
		wb, err := excel.Open(p)
		if err != nil {
			continue
		}
		sheetCount := len(wb.GetSheetList())
		if err := wb.Close(); err != nil {
			t.Errorf("close workbook: %v", err)
		}
		if sheetCount > bestSheets {
			bestSheets = sheetCount
			bestPath = p
		}
	}
	if bestPath == "" {
		t.Fatalf("no matching workbook found in %s", dir)
	}
	return bestPath
}

func findSubDir(t *testing.T, parent, keyword string) string {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("read dir %s: %v", parent, err)
	}
	for _, e := range entries {
		if e.IsDir() && strings.Contains(e.Name(), keyword) {
			return filepath.Join(parent, e.Name())
		}
	}
	t.Fatalf("subdir containing %q not found in %s", keyword, parent)
	return ""
}

func runHistoricalBatchIntegration(
	t *testing.T,
	label string,
	cohortYear int,
	term string,
	examType string,
	mainPath string,
	teacherFallbackPath string,
	fillMissingTiers bool,
) (calc.Result, store.ImportData) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), label+".db"))
	if err != nil {
		t.Fatalf("%s open store: %v", label, err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	ver, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatalf("%s EnsureDefaultPlan: %v", label, err)
	}

	impData, impRep, err := importer.ImportWorkspace(mainPath, importer.Options{ConversionDir: t.TempDir()})
	if err != nil {
		t.Fatalf("%s ImportWorkspace: %v", label, err)
	}
	if len(impData.Assignments) == 0 && teacherFallbackPath != "" {
		tData, _, err := importer.ImportWorkspace(teacherFallbackPath, importer.Options{})
		if err != nil {
			t.Fatalf("%s teacher fallback ImportWorkspace: %v", label, err)
		}
		impData.Assignments = tData.Assignments
		impData.Teachers = tData.Teachers
	}

	if fillMissingTiers && examType != store.ExamGaokao {
		existingTiers := map[string]bool{}
		for _, tc := range impData.TierCutoffs {
			existingTiers[tc.TierKey+":"+tc.Track] = true
		}
		for _, tierKey := range []string{"qingbei", "c9"} {
			for _, track := range []string{"physics", "history"} {
				if !existingTiers[tierKey+":"+track] {
					impData.TierCutoffs = append(impData.TierCutoffs, store.ImportTierCutoff{
						TierKey: tierKey,
						Track:   track,
						Value:   9999,
					})
				}
			}
		}
	}

	cohort, err := s.CreateCohort(ctx, cohortYear, 2025)
	if err != nil {
		t.Fatalf("%s CreateCohort: %v", label, err)
	}
	stage, err := s.GetCohortStage(ctx, cohort.ID, 2025)
	if err != nil {
		t.Fatalf("%s GetCohortStage: %v", label, err)
	}
	for _, req := range []store.StageImportRequest{
		{Kind: "students", Data: impData, Report: store.ImportReport{FileHash: impRep.FileHash + "-stu", Path: label, RowCount: len(impData.Students)}},
		{Kind: "assignments", Data: impData, Report: store.ImportReport{FileHash: impRep.FileHash + "-asg", Path: label, RowCount: len(impData.Assignments)}},
		{Kind: "baselines", Data: impData, Report: store.ImportReport{FileHash: impRep.FileHash + "-bas", Path: label, RowCount: len(impData.ClassBaselines)}},
	} {
		if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, req); err != nil {
			t.Fatalf("%s ApplyStageImport %s: %v", label, req.Kind, err)
		}
	}

	exam, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2025, Term: term, ExamType: examType,
		Name: label, ExamDate: "2026-06-01", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatalf("%s CreateExam: %v", label, err)
	}
	roster := make([]store.ImportRoster, 0, len(impData.Students))
	for _, st := range impData.Students {
		roster = append(roster, store.ImportRoster{
			StudentNo: st.StudentNo, Name: st.Name, ClassName: st.ClassName,
			Track: st.Track, Elective: st.Elective, Status: st.Status, TotalScore: st.TotalScore,
		})
	}
	classes, err := s.ListClasses(ctx, cohort.ID, stage.ID)
	if err != nil {
		t.Fatalf("%s ListClasses: %v", label, err)
	}
	knownClasses := make(map[string]int64, len(classes))
	for _, c := range classes {
		knownClasses[c.Name] = c.ID
	}
	var validTargets []store.ImportClassTarget
	for _, ct := range impData.ClassTargets {
		if _, ok := knownClasses[ct.ClassName]; ok {
			validTargets = append(validTargets, ct)
		}
	}

	for _, req := range []store.ExamImportRequest{
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportRoster, Data: store.ImportData{Roster: roster}, Report: store.ImportReport{FileHash: impRep.FileHash + "-r", Path: label, RowCount: len(roster)}},
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportScores, Data: store.ImportData{Students: impData.Students}, Report: store.ImportReport{FileHash: impRep.FileHash + "-s", Path: label, RowCount: len(impData.Students)}},
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportCutoffs, Data: store.ImportData{SubjectCutoffs: impData.SubjectCutoffs, TierCutoffs: impData.TierCutoffs}, Report: store.ImportReport{FileHash: impRep.FileHash + "-c", Path: label, RowCount: len(impData.SubjectCutoffs)}},
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportTargets, Data: store.ImportData{ClassTargets: validTargets}, Report: store.ImportReport{FileHash: impRep.FileHash + "-t", Path: label, RowCount: len(validTargets)}},
	} {
		if _, err := s.ApplyExamImport(ctx, req); err != nil {
			t.Fatalf("%s ApplyExamImport %s: %v", label, req.Kind, err)
		}
	}

	ec, err := s.GetExamCohort(ctx, exam.ID, cohort.ID)
	if err != nil {
		t.Fatalf("%s GetExamCohort: %v", label, err)
	}
	assess, err := s.AssessmentForExamCohort(ctx, ec.ID)
	if err != nil {
		t.Fatalf("%s AssessmentForExamCohort: %v", label, err)
	}
	if err := s.EnsureAssessmentStatGroups(ctx, assess.ID); err != nil {
		t.Fatalf("%s EnsureAssessmentStatGroups: %v", label, err)
	}

	res, err := pipeline.CalculateAssessment(ctx, s, assess.ID, ver.ID)
	if err != nil {
		if zErr, ok := err.(*calc.ZeroTargetError); ok {
			for _, iss := range zErr.Issues {
				cid, parseErr := strconv.ParseInt(iss.ClassID, 10, 64)
				if parseErr != nil {
					t.Fatalf("%s parse zero target class id %q: %v", label, iss.ClassID, parseErr)
				}
				if err := s.ExcludeAssessmentClass(ctx, assess.ID, cid, "集成核对零目标班级不参评"); err != nil {
					t.Fatalf("%s ExcludeAssessmentClass: %v", label, err)
				}
			}
			res, err = pipeline.CalculateAssessment(ctx, s, assess.ID, ver.ID)
		}
		if err != nil {
			t.Fatalf("%s CalculateAssessment: %v", label, err)
		}
	}
	return res, impData
}

func verifyCompletionAgainstWorkbook(t *testing.T, label, wbPath, sheetKeyword string, res calc.Result) int {
	t.Helper()
	wb, err := excel.Open(wbPath)
	if err != nil {
		t.Fatalf("%s open workbook: %v", label, err)
	}
	defer func() {
		if err := wb.Close(); err != nil {
			t.Errorf("close workbook: %v", err)
		}
	}()

	targetSheet := ""
	for _, s := range wb.GetSheetList() {
		if strings.Contains(s, sheetKeyword) {
			targetSheet = s
			break
		}
	}
	if targetSheet == "" {
		t.Fatalf("%s sheet containing %q not found", label, sheetKeyword)
	}
	tbl, err := excel.ReadTableFromWorkbook(wb, targetSheet)
	if err != nil {
		t.Fatalf("%s read table %s: %v", label, targetSheet, err)
	}
	classCol := excel.FindColumn(tbl.Headers, "班级")
	compCol := -1
	for i, h := range tbl.Headers {
		if strings.Contains(h, "完成率得分") {
			compCol = i
			break
		}
	}
	if classCol < 0 || compCol < 0 {
		t.Fatalf("%s missing class or completion column in %s", label, targetSheet)
	}

	calcComp := map[string]float64{}
	for _, r := range res.Rows {
		if r.CompletionParticipating {
			calcComp[r.LeadClassName] = r.CompletionScore
		}
	}
	matched := 0
	for _, row := range tbl.Rows {
		if classCol >= len(row.Values) || compCol >= len(row.Values) {
			continue
		}
		cn := strings.TrimSpace(row.Values[classCol])
		val, err := strconv.ParseFloat(strings.TrimSpace(row.Values[compCol]), 64)
		if err != nil || cn == "" {
			continue
		}
		got, ok := calcComp[cn]
		if !ok {
			continue
		}
		if math.Abs(got-val) > 1e-3 {
			t.Fatalf("%s completion mismatch: hist=%.6f calc=%.6f", label, val, got)
		}
		matched++
	}
	return matched
}

func TestHistoricalSixBatchesIntegration(t *testing.T) {
	historyDir := resolveHistoryDir(t)
	midDir := findSubDir(t, historyDir, "期中")
	finalDir := findSubDir(t, historyDir, "期末")
	gaokaoDir := findSubDir(t, finalDir, "高考")

	teacherFallback := findMainWorkbook(t, midDir, []string{"教师"}, nil)

	// 1. 高一期中
	g1MidDir := findSubDir(t, midDir, "高一")
	g1MidWb := findMainWorkbook(t, g1MidDir, []string{"量化"}, nil)
	resG1Mid, _ := runHistoricalBatchIntegration(t, "g1_mid", 2025, store.ExamTermSecond, store.ExamMidterm, g1MidWb, teacherFallback, false)
	if len(resG1Mid.Rows) == 0 || len(resG1Mid.Personal) == 0 {
		t.Fatalf("g1_mid produced empty results")
	}
	t.Logf("g1_mid: rows=%d, personal=%d", len(resG1Mid.Rows), len(resG1Mid.Personal))

	// 2. 高二期中
	g2MidDir := findSubDir(t, midDir, "高二")
	g2MidWb := findMainWorkbook(t, g2MidDir, []string{"量化"}, []string{"基础数据"})
	resG2Mid, _ := runHistoricalBatchIntegration(t, "g2_mid", 2024, store.ExamTermSecond, store.ExamMidterm, g2MidWb, teacherFallback, false)
	if len(resG2Mid.Rows) == 0 || len(resG2Mid.Personal) == 0 {
		t.Fatalf("g2_mid produced empty results")
	}
	mG2Mid := verifyCompletionAgainstWorkbook(t, "g2_mid", g2MidWb, "排名、赋分", resG2Mid)
	t.Logf("g2_mid: rows=%d, personal=%d, completion_matched=%d", len(resG2Mid.Rows), len(resG2Mid.Personal), mG2Mid)

	// 3. 高三期中
	g3MidDir := findSubDir(t, midDir, "高三")
	g3MidWb := findMainWorkbook(t, g3MidDir, []string{"量化"}, nil)
	resG3Mid, _ := runHistoricalBatchIntegration(t, "g3_mid", 2023, store.ExamTermSecond, store.ExamMidterm, g3MidWb, teacherFallback, false)
	if len(resG3Mid.Rows) == 0 || len(resG3Mid.Personal) == 0 {
		t.Fatalf("g3_mid produced empty results")
	}
	mG3Mid := verifyCompletionAgainstWorkbook(t, "g3_mid", g3MidWb, "排名、赋分", resG3Mid)
	t.Logf("g3_mid: rows=%d, personal=%d, completion_matched=%d", len(resG3Mid.Rows), len(resG3Mid.Personal), mG3Mid)

	// 4. 高一期末
	g1FinWb := findMainWorkbook(t, finalDir, []string{"高一期末"}, nil)
	resG1Fin, _ := runHistoricalBatchIntegration(t, "g1_final", 2025, store.ExamTermSecond, store.ExamFinal, g1FinWb, "", true)
	if len(resG1Fin.Rows) == 0 || len(resG1Fin.Personal) == 0 {
		t.Fatalf("g1_final produced empty results")
	}
	t.Logf("g1_final: rows=%d, personal=%d", len(resG1Fin.Rows), len(resG1Fin.Personal))

	// 5. 高二期末
	g2FinWb := findMainWorkbook(t, finalDir, []string{"高二期末"}, nil)
	resG2Fin, _ := runHistoricalBatchIntegration(t, "g2_final", 2024, store.ExamTermSecond, store.ExamFinal, g2FinWb, "", true)
	if len(resG2Fin.Rows) == 0 || len(resG2Fin.Personal) == 0 {
		t.Fatalf("g2_final produced empty results")
	}
	mG2Fin := verifyCompletionAgainstWorkbook(t, "g2_final", g2FinWb, "排名、赋分", resG2Fin)
	if mG2Fin != 24 {
		t.Fatalf("g2_final expected 24 completion matches, got %d", mG2Fin)
	}
	t.Logf("g2_final: rows=%d, personal=%d, completion_matched=%d", len(resG2Fin.Rows), len(resG2Fin.Personal), mG2Fin)

	// 6. 2026 高考
	gaokaoWb := findMainWorkbook(t, gaokaoDir, nil, []string{"应届", "成绩量化"})
	resGaokao, _ := runHistoricalBatchIntegration(t, "gaokao", 2023, store.ExamTermSecond, store.ExamGaokao, gaokaoWb, "", false)
	if len(resGaokao.Rows) == 0 || len(resGaokao.Personal) == 0 {
		t.Fatalf("gaokao produced empty results")
	}
	mGaokao := verifyCompletionAgainstWorkbook(t, "gaokao", gaokaoWb, "教学成绩量化-新", resGaokao)
	t.Logf("gaokao: rows=%d, personal=%d, completion_matched=%d", len(resGaokao.Rows), len(resGaokao.Personal), mGaokao)

	// 汇总输出排序键，确保测试过程有明确统计。
	keys := make([]string, 0, len(resGaokao.Rows))
	for _, r := range resGaokao.Rows {
		keys = append(keys, r.Subject)
	}
	sort.Strings(keys)
}
