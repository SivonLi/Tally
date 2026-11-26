//go:build integration

package pipeline_test

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
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
	var candidates []string
	if envDir := strings.TrimSpace(os.Getenv("TALLY_PRIVATE_DATA_DIR")); envDir != "" {
		candidates = append(candidates, envDir)
	}
	if wd, err := os.Getwd(); err == nil {
		cur := wd
		for i := 0; i < 6; i++ {
			candidates = append(candidates, filepath.Join(cur, "private-data", "history"))
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
		}
	}
	if out, err := exec.Command("git", "rev-parse", "--git-common-dir").Output(); err == nil {
		commonDir := strings.TrimSpace(string(out))
		if commonDir != "" {
			if !filepath.IsAbs(commonDir) {
				if abs, err := filepath.Abs(commonDir); err == nil {
					commonDir = abs
				}
			}
			mainRoot := filepath.Dir(commonDir)
			candidates = append(candidates, filepath.Join(mainRoot, "private-data", "history"))
		}
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

func verifyG1FinalMetrics(t *testing.T, wbPath string, res calc.Result) int {
	t.Helper()
	wb, err := excel.Open(wbPath)
	if err != nil {
		t.Fatalf("g1_final open workbook: %v", err)
	}
	defer func() {
		if err := wb.Close(); err != nil {
			t.Errorf("close workbook: %v", err)
		}
	}()

	rankSheet := ""
	for _, s := range wb.GetSheetList() {
		if strings.Contains(s, "排名、赋分") {
			rankSheet = s
			break
		}
	}
	if rankSheet == "" {
		t.Fatalf("g1_final ranking sheet not found")
	}
	tbl, err := excel.ReadTableFromWorkbook(wb, rankSheet)
	if err != nil {
		t.Fatalf("g1_final read ranking sheet: %v", err)
	}
	classCol := excel.FindColumn(tbl.Headers, "班级")
	if classCol < 0 {
		t.Fatalf("g1_final missing class column")
	}
	calcImp := map[string]float64{}
	passTotals := map[string]int{}
	for _, r := range res.Rows {
		calcImp[r.Subject+":"+r.LeadClassName] = r.ImprovementScore
		passTotals[r.Subject] += r.PassCount
	}

	subjects := []struct {
		name string
		code string
		want int
	}{
		{"数学", "math", 22},
		{"英语", "english", 22},
		{"物理", "physics", 18},
		{"化学", "chemistry", 18},
		{"生物", "biology", 13},
	}
	totalMatched := 0
	for _, sub := range subjects {
		subIdx := -1
		for i, h := range tbl.Headers {
			trimmed := strings.TrimSpace(h)
			if trimmed == sub.name || (sub.name == "英语" && trimmed == "外语") {
				subIdx = i
				break
			}
		}
		if subIdx < 0 {
			t.Fatalf("g1_final missing subject header for %s", sub.code)
		}
		matched := 0
		for _, row := range tbl.Rows {
			if classCol >= len(row.Values) || subIdx+3 >= len(row.Values) {
				continue
			}
			cn := strings.TrimSpace(row.Values[classCol])
			if cn == "" || cn == "班级" || strings.Contains(cn, "总计") {
				continue
			}
			hImp, err := strconv.ParseFloat(strings.TrimSpace(row.Values[subIdx+3]), 64)
			if err != nil {
				continue
			}
			got, ok := calcImp[sub.code+":"+cn]
			if !ok {
				continue
			}
			if math.Abs(got-hImp) > 0.02 {
				t.Fatalf("g1_final %s improvement mismatch: hist=%.4f calc=%.4f", sub.code, hImp, got)
			}
			matched++
		}
		if matched != sub.want {
			t.Fatalf("g1_final %s improvement matched=%d, want %d", sub.code, matched, sub.want)
		}
		totalMatched += matched
	}

	expectedPassTotals := map[string]int{
		"chinese": 363,
		"math":    427,
		"english": 339,
		"biology": 256,
	}
	for subj, want := range expectedPassTotals {
		if passTotals[subj] != want {
			t.Fatalf("g1_final %s pass total=%d, want %d", subj, passTotals[subj], want)
		}
	}
	return totalMatched
}

func verifyChemistryAllMetrics(t *testing.T, label, wbPath, sheetKeyword string, res calc.Result, want int) int {
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
		t.Fatalf("%s read table: %v", label, err)
	}
	classCol := excel.FindColumn(tbl.Headers, "班级")
	subIdx := -1
	for i, h := range tbl.Headers {
		if strings.TrimSpace(h) == "化学" {
			subIdx = i
			break
		}
	}
	if classCol < 0 || subIdx < 0 {
		t.Fatalf("%s missing class or chemistry column", label)
	}
	byClass := map[string]calc.Row{}
	for _, r := range res.Rows {
		if r.Subject == "chemistry" {
			byClass[r.LeadClassName] = r
		}
	}
	matched := 0
	for _, row := range tbl.Rows {
		if classCol >= len(row.Values) || subIdx+8 >= len(row.Values) {
			continue
		}
		cn := strings.TrimSpace(row.Values[classCol])
		got, ok := byClass[cn]
		if !ok {
			continue
		}
		hTot, errT := strconv.ParseFloat(strings.TrimSpace(row.Values[subIdx+5]), 64)
		if errT != nil {
			continue
		}
		hImp, err := strconv.ParseFloat(strings.TrimSpace(row.Values[subIdx+3]), 64)
		if err != nil {
			t.Fatalf("%s chemistry parse imp: %v", label, err)
		}
		hPass, err := strconv.ParseFloat(strings.TrimSpace(row.Values[subIdx+4]), 64)
		if err != nil {
			t.Fatalf("%s chemistry parse pass: %v", label, err)
		}
		hRank, err := strconv.Atoi(strings.TrimSpace(row.Values[subIdx+6]))
		if err != nil {
			t.Fatalf("%s chemistry parse rank: %v", label, err)
		}
		hTier, err := strconv.Atoi(strings.TrimSpace(row.Values[subIdx+7]))
		if err != nil {
			t.Fatalf("%s chemistry parse tier: %v", label, err)
		}
		hPts, err := strconv.ParseFloat(strings.TrimSpace(row.Values[subIdx+8]), 64)
		if err != nil {
			t.Fatalf("%s chemistry parse points: %v", label, err)
		}
		if math.Abs(got.ImprovementScore-hImp) > 0.02 ||
			math.Abs(got.PassScore-hPass) > 0.02 ||
			math.Abs(got.Total-hTot) > 0.02 ||
			got.Rank != hRank ||
			(got.TierIndex+1) != hTier ||
			math.Abs(got.Points-hPts) > 0.01 {
			t.Fatalf("%s chemistry mismatch: hist(imp=%.4f,pass=%.4f,tot=%.4f,r=%d,t=%d,p=%.1f) vs calc(imp=%.4f,pass=%.4f,tot=%.4f,r=%d,t=%d,p=%.1f)",
				label, hImp, hPass, hTot, hRank, hTier, hPts,
				got.ImprovementScore, got.PassScore, got.Total, got.Rank, got.TierIndex+1, got.Points)
		}
		matched++
	}
	if matched != want {
		t.Fatalf("%s chemistry matched=%d, want %d", label, matched, want)
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
	if len(resG1Mid.Rows) != 141 || len(resG1Mid.Personal) != 73 {
		t.Fatalf("g1_mid unexpected counts: rows=%d, personal=%d", len(resG1Mid.Rows), len(resG1Mid.Personal))
	}
	mG1Mid := verifyCompletionAgainstWorkbook(t, "g1_mid", g1MidWb, "排名、赋分", resG1Mid)
	if mG1Mid != 22 {
		t.Fatalf("g1_mid expected 22 completion matches, got %d", mG1Mid)
	}
	t.Logf("g1_mid: rows=%d, personal=%d, completion_matched=%d", len(resG1Mid.Rows), len(resG1Mid.Personal), mG1Mid)

	// 2. 高二期中
	g2MidDir := findSubDir(t, midDir, "高二")
	g2MidWb := findMainWorkbook(t, g2MidDir, []string{"量化"}, []string{"基础数据"})
	resG2Mid, _ := runHistoricalBatchIntegration(t, "g2_mid", 2024, store.ExamTermSecond, store.ExamMidterm, g2MidWb, teacherFallback, false)
	if len(resG2Mid.Rows) != 154 || len(resG2Mid.Personal) != 83 {
		t.Fatalf("g2_mid unexpected counts: rows=%d, personal=%d", len(resG2Mid.Rows), len(resG2Mid.Personal))
	}
	mG2Mid := verifyCompletionAgainstWorkbook(t, "g2_mid", g2MidWb, "排名、赋分", resG2Mid)
	if mG2Mid != 24 {
		t.Fatalf("g2_mid expected 24 completion matches, got %d", mG2Mid)
	}
	t.Logf("g2_mid: rows=%d, personal=%d, completion_matched=%d", len(resG2Mid.Rows), len(resG2Mid.Personal), mG2Mid)

	// 3. 高三期中
	g3MidDir := findSubDir(t, midDir, "高三")
	g3MidWb := findMainWorkbook(t, g3MidDir, []string{"量化"}, nil)
	resG3Mid, _ := runHistoricalBatchIntegration(t, "g3_mid", 2023, store.ExamTermSecond, store.ExamMidterm, g3MidWb, teacherFallback, false)
	if len(resG3Mid.Rows) != 106 || len(resG3Mid.Personal) != 60 {
		t.Fatalf("g3_mid unexpected counts: rows=%d, personal=%d", len(resG3Mid.Rows), len(resG3Mid.Personal))
	}
	mG3Mid := verifyCompletionAgainstWorkbook(t, "g3_mid", g3MidWb, "排名、赋分", resG3Mid)
	if mG3Mid != 17 {
		t.Fatalf("g3_mid expected 17 completion matches, got %d", mG3Mid)
	}
	t.Logf("g3_mid: rows=%d, personal=%d, completion_matched=%d", len(resG3Mid.Rows), len(resG3Mid.Personal), mG3Mid)

	// 4. 高一期末
	g1FinWb := findMainWorkbook(t, finalDir, []string{"高一期末"}, nil)
	resG1Fin, _ := runHistoricalBatchIntegration(t, "g1_final", 2025, store.ExamTermSecond, store.ExamFinal, g1FinWb, "", true)
	if len(resG1Fin.Rows) != 141 || len(resG1Fin.Personal) != 73 {
		t.Fatalf("g1_final unexpected counts: rows=%d, personal=%d", len(resG1Fin.Rows), len(resG1Fin.Personal))
	}
	mG1FinImp := verifyG1FinalMetrics(t, g1FinWb, resG1Fin)
	t.Logf("g1_final: rows=%d, personal=%d, core_imp_matched=%d", len(resG1Fin.Rows), len(resG1Fin.Personal), mG1FinImp)

	// 5. 高二期末
	g2FinWb := findMainWorkbook(t, finalDir, []string{"高二期末"}, nil)
	resG2Fin, _ := runHistoricalBatchIntegration(t, "g2_final", 2024, store.ExamTermSecond, store.ExamFinal, g2FinWb, "", true)
	if len(resG2Fin.Rows) != 154 || len(resG2Fin.Personal) != 83 {
		t.Fatalf("g2_final unexpected counts: rows=%d, personal=%d", len(resG2Fin.Rows), len(resG2Fin.Personal))
	}
	mG2Fin := verifyCompletionAgainstWorkbook(t, "g2_final", g2FinWb, "排名、赋分", resG2Fin)
	if mG2Fin != 24 {
		t.Fatalf("g2_final expected 24 completion matches, got %d", mG2Fin)
	}
	mG2Chem := verifyChemistryAllMetrics(t, "g2_final", g2FinWb, "排名、赋分", resG2Fin, 22)
	t.Logf("g2_final: rows=%d, personal=%d, completion_matched=%d, chemistry_full_matched=%d",
		len(resG2Fin.Rows), len(resG2Fin.Personal), mG2Fin, mG2Chem)

	// 6. 2026 高考
	gaokaoWb := findMainWorkbook(t, gaokaoDir, nil, []string{"应届", "成绩量化"})
	resGaokao, _ := runHistoricalBatchIntegration(t, "gaokao", 2023, store.ExamTermSecond, store.ExamGaokao, gaokaoWb, "", false)
	if len(resGaokao.Rows) != 103 || len(resGaokao.Personal) != 69 {
		t.Fatalf("gaokao unexpected counts: rows=%d, personal=%d", len(resGaokao.Rows), len(resGaokao.Personal))
	}
	mGaokao := verifyCompletionAgainstWorkbook(t, "gaokao", gaokaoWb, "教学成绩量化-新", resGaokao)
	if mGaokao != 30 {
		t.Fatalf("gaokao expected 30 completion matches, got %d", mGaokao)
	}
	t.Logf("gaokao: rows=%d, personal=%d, completion_matched=%d", len(resGaokao.Rows), len(resGaokao.Personal), mGaokao)
}
