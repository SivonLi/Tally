package importer

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"tally/internal/store"
)

func archiveTestSubjects() []store.Subject {
	return []store.Subject{
		{Key: "chinese", Name: "语文", FullMark: 150, Enabled: true},
		{Key: "math", Name: "数学", FullMark: 150, Enabled: true},
		{Key: "english", Name: "英语", FullMark: 150, Enabled: true},
	}
}

func TestInspectArchiveWorkbookLongStandard(t *testing.T) {
	path := filepath.Join("testdata", "archive_long_standard.xlsx")
	res, err := InspectArchiveWorkbook(path, Options{}, archiveTestSubjects())
	if err != nil {
		t.Fatalf("InspectArchiveWorkbook failed: %v", err)
	}

	if res.FileHash == "" {
		t.Fatalf("expected non-empty FileHash")
	}

	roleBySheet := map[string]ArchiveSheetInfo{}
	for _, s := range res.Sheets {
		roleBySheet[s.Name] = s
	}
	if roleBySheet["说明"].Role != "" || roleBySheet["说明"].Recommended {
		t.Fatalf("expected distractor sheet ignored, got %+v", roleBySheet["说明"])
	}
	if roleBySheet["结果A"].Role != ArchiveRoleQuant || !roleBySheet["结果A"].Recommended {
		t.Fatalf("expected 结果A recognized by headers as ArchiveRoleQuant, got %+v", roleBySheet["结果A"])
	}
	if roleBySheet["结果B"].Role != ArchiveRoleRank || !roleBySheet["结果B"].Recommended {
		t.Fatalf("expected 结果B recognized by headers as ArchiveRoleRank, got %+v", roleBySheet["结果B"])
	}
	if roleBySheet["结果C"].Role != ArchiveRolePersonal || !roleBySheet["结果C"].Recommended {
		t.Fatalf("expected 结果C recognized by headers as ArchiveRolePersonal, got %+v", roleBySheet["结果C"])
	}

	for _, iss := range res.Issues {
		if iss.Severity == "blocking" {
			t.Fatalf("expected no blocking issues, got %+v", iss)
		}
	}

	if len(res.Data.Rows) != 3 {
		t.Fatalf("expected 3 merged result rows, got %d (%+v)", len(res.Data.Rows), res.Data.Rows)
	}
	if len(res.Data.Personal) != 2 {
		t.Fatalf("expected 2 personal scores (excluding 合计 row), got %d (%+v)", len(res.Data.Personal), res.Data.Personal)
	}

	rowMap := map[string]store.ImportArchivedResultRow{}
	for _, r := range res.Data.Rows {
		rowMap[r.ClassName+"|"+r.Subject] = r
	}
	r1, ok := rowMap["1班|chinese"]
	if !ok {
		t.Fatalf("missing 1班|chinese merged row")
	}
	if len(r1.TeacherNames) != 1 || r1.TeacherNames[0] != "张老师" || r1.Total != 88.6 || r1.Rank != 2 || r1.TierIndex == nil || *r1.TierIndex != 0 || r1.Points != 45.0 {
		t.Fatalf("unexpected merged row 1班|chinese: %+v", r1)
	}
	if r1.ImprovementScore != 39.2 || r1.PassScore != 39.4 || r1.CompletionScore != 10.0 {
		t.Fatalf("unexpected three sub-scores for 1班|chinese: %+v", r1)
	}
	if r1.ClassAverage != 118.5 || r1.GradeAverage != 112.0 || r1.BaselineAverage != 110.0 || r1.PassCount != 42 || r1.PassTarget != 40 {
		t.Fatalf("unexpected auxiliary stats for 1班|chinese: %+v", r1)
	}

	if res.Data.Personal[0].TeacherName != "张老师" || res.Data.Personal[0].Subject != "chinese" || res.Data.Personal[0].Score != 47.5 {
		t.Fatalf("unexpected personal score[0]: %+v", res.Data.Personal[0])
	}
	if len(res.Data.Personal[0].GroupPoints) != 2 || res.Data.Personal[0].GroupPoints[0] != 45 || res.Data.Personal[0].GroupPoints[1] != 50 {
		t.Fatalf("unexpected group points: %+v", res.Data.Personal[0].GroupPoints)
	}
}

func TestInspectArchiveWorkbookWideAndMergedCells(t *testing.T) {
	path := filepath.Join("testdata", "archive_wide_merged.xlsx")
	res, err := InspectArchiveWorkbook(path, Options{}, archiveTestSubjects())
	if err != nil {
		t.Fatalf("InspectArchiveWorkbook wide failed: %v", err)
	}

	roleBySheet := map[string]ArchiveSheetInfo{}
	for _, s := range res.Sheets {
		roleBySheet[s.Name] = s
	}
	if roleBySheet["宽表量化"].Role != ArchiveRoleQuant || roleBySheet["宽表量化"].Format != "wide" {
		t.Fatalf("unexpected quant sheet classification: %+v", roleBySheet["宽表量化"])
	}
	if roleBySheet["宽表排名赋分"].Role != ArchiveRoleRank || roleBySheet["宽表排名赋分"].Format != "wide" {
		t.Fatalf("unexpected rank sheet classification: %+v", roleBySheet["宽表排名赋分"])
	}
	if roleBySheet["个人得分"].Role != ArchiveRolePersonal {
		t.Fatalf("unexpected personal sheet classification: %+v", roleBySheet["个人得分"])
	}
	if roleBySheet["学生成绩单"].Role != "" {
		t.Fatalf("expected student exam-no sheet to be excluded, got %+v", roleBySheet["学生成绩单"])
	}

	if len(res.Data.Rows) != 6 {
		t.Fatalf("expected 6 wide result rows (3 classes x 2 subjects), got %d", len(res.Data.Rows))
	}

	rowMap := map[string]store.ImportArchivedResultRow{}
	for _, r := range res.Data.Rows {
		rowMap[r.ClassName+"|"+r.Subject] = r
	}
	// 验证 宽表量化 中 G5:G6 合并单元格（陈老师）按左上角取值展开到 3班|math
	r3Math, ok := rowMap["3班|math"]
	if !ok {
		t.Fatalf("missing 3班|math row")
	}
	if len(r3Math.TeacherNames) != 1 || r3Math.TeacherNames[0] != "陈老师" || r3Math.Total != 81.5 || r3Math.Rank != 3 || r3Math.TierIndex == nil || *r3Math.TierIndex != 2 || r3Math.Points != 35.0 {
		t.Fatalf("unexpected merged cell row 3班|math: %+v", r3Math)
	}
	// 验证 2档 / 第三档 等归一化（0-based: 二档 -> 1, 三档 -> 2）
	r1Chi := rowMap["1班|chinese"]
	r3Chi := rowMap["3班|chinese"]
	if r1Chi.TierIndex == nil || *r1Chi.TierIndex != 1 || r3Chi.TierIndex == nil || *r3Chi.TierIndex != 2 {
		t.Fatalf("unexpected tier normalization: 1班=%v 3班=%v", r1Chi.TierIndex, r3Chi.TierIndex)
	}

	// 验证右侧汇总块提取了 4 条个人得分（包括 孙老师 35.0）
	if len(res.Data.Personal) != 4 {
		t.Fatalf("expected 4 personal scores from summary block, got %d (%+v)", len(res.Data.Personal), res.Data.Personal)
	}
	foundSun := false
	for _, p := range res.Data.Personal {
		if p.TeacherName == "孙老师" && p.Subject == "chinese" && p.Score == 35.0 {
			foundSun = true
		}
	}
	if !foundSun {
		t.Fatalf("expected right-hand summary block to include 孙老师 35.0, got %+v", res.Data.Personal)
	}
}

func TestInspectArchiveWorkbookIssuesAndRoundTripStore(t *testing.T) {
	issuesPath := filepath.Join("testdata", "archive_issues.xlsx")
	res, err := InspectArchiveWorkbook(issuesPath, Options{}, archiveTestSubjects())
	if err != nil {
		t.Fatalf("InspectArchiveWorkbook issues failed: %v", err)
	}

	countsByCode := map[string]string{}
	missingClass, missingTeacher := false, false
	for _, iss := range res.Issues {
		countsByCode[iss.Code] = iss.Severity
		if iss.Code == "missing_field" && iss.Column == "class_name" && iss.Severity == "blocking" {
			missingClass = true
		}
		if iss.Code == "missing_field" && iss.Column == "teacher_name" && iss.Severity == "blocking" {
			missingTeacher = true
		}
	}
	if !missingClass || !missingTeacher {
		t.Fatalf("expected blocking missing_field for both class_name and teacher_name, got %+v", res.Issues)
	}
	for code, wantSev := range map[string]string{
		"missing_field":          "blocking",
		"unknown_subject":        "blocking",
		"invalid_number":         "blocking",
		"archive_invalid_tier":   "warning",
		"archive_missing_points": "warning",
		"archive_missing_total":  "warning",
	} {
		gotSev, ok := countsByCode[code]
		if !ok {
			t.Fatalf("expected issue code %s, got issues=%+v", code, res.Issues)
		}
		if code == "invalid_number" {
			hasBlock, hasWarn := false, false
			for _, iss := range res.Issues {
				if iss.Code == "invalid_number" {
					if iss.Severity == "blocking" {
						hasBlock = true
					}
					if iss.Severity == "warning" {
						hasWarn = true
					}
				}
			}
			if !hasBlock || !hasWarn {
				t.Fatalf("expected both blocking and warning invalid_number, got %+v", res.Issues)
			}
			continue
		}
		if gotSev != wantSev {
			t.Fatalf("issue code %s: expected severity %s, got %s", code, wantSev, gotSev)
		}
	}

	// 验证标准工作簿可直接写入真实 SQLite store，并触发只读保护与 Archived 状态查询。
	longPath := filepath.Join("testdata", "archive_long_standard.xlsx")
	cleanRes, err := InspectArchiveWorkbook(longPath, Options{}, archiveTestSubjects())
	if err != nil {
		t.Fatalf("InspectArchiveWorkbook clean failed: %v", err)
	}

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive_roundtrip.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer func() {
		if closeErr := st.Close(); closeErr != nil {
			t.Errorf("st.Close failed: %v", closeErr)
		}
	}()

	cohort, err := st.CreateCohort(ctx, 2025, 2025)
	if err != nil {
		t.Fatalf("CreateCohort failed: %v", err)
	}
	cohortID := cohort.ID

	exam, err := st.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2025,
		Term:              store.ExamTermFirst,
		ExamType:          store.ExamMidterm,
		Name:              "合成归档测试考试",
		ExamDate:          "2026-04-20",
		CohortIDs:         []int64{cohortID},
	})
	if err != nil {
		t.Fatalf("CreateExam failed: %v", err)
	}
	ec, err := st.GetExamCohort(ctx, exam.ID, cohortID)
	if err != nil {
		t.Fatalf("GetExamCohort failed: %v", err)
	}
	assessment, err := st.AssessmentForExamCohort(ctx, ec.ID)
	if err != nil {
		t.Fatalf("AssessmentForExamCohort failed: %v", err)
	}

	req := store.ArchivedResultImportRequest{
		AssessmentID: assessment.ID,
		Data:         cleanRes.Data,
		Report:       cleanRes.Report,
		Issues:       cleanRes.Issues,
	}
	preview, err := st.ValidateArchivedResultImport(ctx, req)
	if err != nil {
		t.Fatalf("ValidateArchivedResultImport failed: %v", err)
	}
	for _, iss := range preview.Issues {
		if iss.Severity == "blocking" {
			t.Fatalf("expected 0 blocking store validation issues, got %+v", preview.Issues)
		}
	}

	summary, err := st.ApplyArchivedResultImport(ctx, req)
	if err != nil {
		t.Fatalf("ApplyArchivedResultImport failed: %v", err)
	}
	if summary.ResultRows != 3 || summary.PersonalScores != 2 || summary.ImportRunID == 0 {
		t.Fatalf("unexpected apply summary: %+v", summary)
	}

	loaded, err := st.LoadAssessmentContext(ctx, assessment.ID)
	if err != nil {
		t.Fatalf("LoadAssessmentContext failed: %v", err)
	}
	if !loaded.Assessment.Archived {
		t.Fatalf("expected loaded assessment Archived=true")
	}
	if err := st.AssertAssessmentCalculable(ctx, assessment.ID); err != store.ErrAssessmentArchived {
		t.Fatalf("expected ErrAssessmentArchived, got %v", err)
	}
}

func TestInspectArchiveWorkbookWidePrefixAndPartialRankMerge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive_partial_merge.xlsx")
	wb := excelize.NewFile()
	defer func() {
		if err := wb.Close(); err != nil {
			t.Errorf("close workbook: %v", err)
		}
	}()

	// 1. 量化计算（宽表）：首列学科块从「年级均分」开始（不带「任课教师」首列），且带部分「排名」列。
	qSheet := "量化计算"
	if err := wb.SetSheetName("Sheet1", qSheet); err != nil {
		t.Fatal(err)
	}
	qRows := [][]any{
		{"班级", "参考人数", "完成率得分", "语文", "语文", "语文", "语文", "语文", "语文", "语文", "语文", "语文", "语文", "语文", "语文"},
		{"班级", "参考人数", "完成率得分", "年级均分", "班级均分", "基数均分", "基数比值", "提升率未封顶值", "均分提升率得分", "达线人数", "单科目标", "达线率未封顶值", "单科达线率得分", "教学量化成绩", "排名"},
		{"1班", 50, 12.5, 108.0, 115.0, 105.0, 1.095, 0.0648, 38.2, 40, 36.5, 1.0959, 41.3, 92.0, 1},
		{"2班", 48, 10.0, 108.0, 104.0, 102.0, 1.019, 0.0196, 30.0, 32, 35.0, 0.9143, 35.0, 75.0, 2},
	}
	for rIdx, row := range qRows {
		for cIdx, val := range row {
			cell, err := excelize.CoordinatesToCellName(cIdx+1, rIdx+1)
			if err != nil {
				t.Fatal(err)
			}
			if err := wb.SetCellValue(qSheet, cell, val); err != nil {
				t.Fatal(err)
			}
		}
	}

	// 2. 排名赋分（长表）：补齐任课教师、档次、赋分，且 2班 故意缺少赋分用于检验 archive_missing_points 仅报一次。
	rSheet := "排名赋分"
	if _, err := wb.NewSheet(rSheet); err != nil {
		t.Fatal(err)
	}
	rRows := [][]any{
		{"班级", "学科", "任课教师", "教学量化成绩", "排名", "档次", "赋分"},
		{"1班", "语文", "赵老师", 92.0, 1, "一档", 45.0},
		{"2班", "语文", "钱老师", 75.0, 2, "二档", ""},
	}
	for rIdx, row := range rRows {
		for cIdx, val := range row {
			cell, err := excelize.CoordinatesToCellName(cIdx+1, rIdx+1)
			if err != nil {
				t.Fatal(err)
			}
			if err := wb.SetCellValue(rSheet, cell, val); err != nil {
				t.Fatal(err)
			}
		}
	}

	// 3. 个人得分表。
	pSheet := "个人得分"
	if _, err := wb.NewSheet(pSheet); err != nil {
		t.Fatal(err)
	}
	pRows := [][]any{
		{"学科", "教师", "个人得分"},
		{"语文", "赵老师", 45.0},
	}
	for rIdx, row := range pRows {
		for cIdx, val := range row {
			cell, err := excelize.CoordinatesToCellName(cIdx+1, rIdx+1)
			if err != nil {
				t.Fatal(err)
			}
			if err := wb.SetCellValue(pSheet, cell, val); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := wb.SaveAs(path); err != nil {
		t.Fatal(err)
	}

	res, err := InspectArchiveWorkbook(path, Options{}, archiveTestSubjects())
	if err != nil {
		t.Fatalf("InspectArchiveWorkbook: %v", err)
	}
	for _, iss := range res.Issues {
		if iss.Severity == "blocking" {
			t.Fatalf("unexpected blocking issue: %+v", iss)
		}
	}
	if len(res.Data.Rows) != 2 {
		t.Fatalf("expected 2 merged rows without duplicate extras, got %d (%+v)", len(res.Data.Rows), res.Data.Rows)
	}

	rowMap := map[string]store.ImportArchivedResultRow{}
	for _, row := range res.Data.Rows {
		rowMap[row.ClassName] = row
	}
	r1 := rowMap["1班"]
	if r1.ClassCount != 50 || r1.GradeAverage != 108.0 || r1.ClassAverage != 115.0 ||
		r1.BaselineAverage != 105.0 || r1.BaselineRatio != 1.095 ||
		r1.ImprovementScore != 38.2 || r1.PassCount != 40 || r1.PassTarget != 36.5 || r1.PassScore != 41.3 ||
		!r1.CompletionParticipate || r1.CompletionScore != 12.5 || r1.Total != 92.0 ||
		r1.Rank != 1 || r1.TierIndex == nil || *r1.TierIndex != 0 || r1.Points != 45.0 ||
		len(r1.TeacherNames) != 1 || r1.TeacherNames[0] != "赵老师" {
		t.Fatalf("merged row 1班 lost fields: %+v", r1)
	}

	missingPts := 0
	for _, iss := range res.Issues {
		if iss.Code == "archive_missing_points" {
			missingPts++
		}
	}
	if missingPts != 1 {
		t.Fatalf("expected archive_missing_points once for 2班, got %d (%+v)", missingPts, res.Issues)
	}
}

func TestInspectArchiveWorkbookLongPartialRankMergeAndFieldRetention(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive_long_partial_merge.xlsx")
	wb := excelize.NewFile()
	defer func() {
		if err := wb.Close(); err != nil {
			t.Errorf("close workbook: %v", err)
		}
	}()

	qSheet := "量化计算"
	if err := wb.SetSheetName("Sheet1", qSheet); err != nil {
		t.Fatal(err)
	}
	qRows := [][]any{
		{"班级", "学科", "任课教师", "参考人数", "班级均分", "年级均分", "基数均分", "基数比值", "提升率未封顶值", "均分提升率得分", "达线人数", "单科目标", "达线率未封顶值", "单科达线率得分", "完成率得分", "教学量化成绩", "排名"},
		{"1班", "数学", "孙老师", 46, 112.5, 101.0, 100.0, 1.125, 0.1139, 44.0, 38, 34.0, 1.1176, 40.5, 11.5, 96.0, 1},
	}
	for rIdx, row := range qRows {
		for cIdx, val := range row {
			cell, err := excelize.CoordinatesToCellName(cIdx+1, rIdx+1)
			if err != nil {
				t.Fatal(err)
			}
			if err := wb.SetCellValue(qSheet, cell, val); err != nil {
				t.Fatal(err)
			}
		}
	}

	rSheet := "排名赋分"
	if _, err := wb.NewSheet(rSheet); err != nil {
		t.Fatal(err)
	}
	rRows := [][]any{
		{"班级", "学科", "排名", "档次", "赋分"},
		{"1班", "数学", 1, "一档", 48.0},
	}
	for rIdx, row := range rRows {
		for cIdx, val := range row {
			cell, err := excelize.CoordinatesToCellName(cIdx+1, rIdx+1)
			if err != nil {
				t.Fatal(err)
			}
			if err := wb.SetCellValue(rSheet, cell, val); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := wb.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	res, err := InspectArchiveWorkbook(path, Options{}, archiveTestSubjects())
	if err != nil {
		t.Fatalf("InspectArchiveWorkbook: %v", err)
	}
	if len(res.Data.Rows) != 1 {
		t.Fatalf("expected 1 merged row, got %d (%+v)", len(res.Data.Rows), res.Data.Rows)
	}
	r := res.Data.Rows[0]
	if r.ImprovementRaw != 0.1139 || r.PassRaw != 1.1176 || r.Rank != 1 || r.TierIndex == nil || *r.TierIndex != 0 || r.Points != 48.0 {
		t.Fatalf("expected all raw and rank fields retained after merge: %+v", r)
	}
}
