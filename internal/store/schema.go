package store

import "github.com/egoist/mygo/plugins/sqlite"

func schemaV1Statements() []sqlite.Statement {
	return []sqlite.Statement{
		// 2.1 全校永久配置与师资库
		{SQL: `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS app_settings (
			"key" TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS subjects (
			"key" TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			full_mark REAL NOT NULL,
			sort_order INTEGER NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS tracks (
			"key" TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			sort_order INTEGER NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS plans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			scope TEXT NOT NULL DEFAULT 'school',
			is_system INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS plan_versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			plan_id INTEGER NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
			version INTEGER NOT NULL,
			status TEXT NOT NULL CHECK(status IN ('draft','published','retired')),
			config_json TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL DEFAULT '',
			parent_version_id INTEGER,
			change_summary TEXT NOT NULL DEFAULT '',
			config_hash TEXT NOT NULL DEFAULT '',
			published_at TEXT,
			created_at TEXT NOT NULL,
			UNIQUE(plan_id, version)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS sync_state (
			"key" TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS saved_views (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			page TEXT NOT NULL,
			name TEXT NOT NULL,
			config_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE(page, name)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS teachers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			teacher_no TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			primary_subject TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','inactive')),
			note TEXT NOT NULL DEFAULT '',
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`},
		{SQL: `CREATE UNIQUE INDEX IF NOT EXISTS idx_teachers_no
			ON teachers(teacher_no) WHERE teacher_no <> ''`},
		{SQL: `CREATE UNIQUE INDEX IF NOT EXISTS idx_teachers_name_subj
			ON teachers(name, primary_subject) WHERE teacher_no = ''`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_teachers_name
			ON teachers(name)`},

		// 2.2 年级三年生命周期
		{SQL: `CREATE TABLE IF NOT EXISTS cohorts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			enrollment_year INTEGER NOT NULL UNIQUE,
			graduation_year INTEGER NOT NULL,
			current_phase TEXT NOT NULL CHECK(current_phase IN ('g1','g2','g3','graduated')),
			current_academic_year_start INTEGER NOT NULL,
			graduated_at TEXT,
			created_at TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS classes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id) ON DELETE CASCADE,
			phase TEXT NOT NULL CHECK(phase IN ('g1','g2','g3')),
			name TEXT NOT NULL,
			continued_from_class_id INTEGER REFERENCES classes(id),
			track_hint TEXT NOT NULL DEFAULT '',
			room TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','archived','graduated')),
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			UNIQUE(cohort_id, phase, name)
		)`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_classes_continued
			ON classes(continued_from_class_id)`},
		{SQL: `CREATE TABLE IF NOT EXISTS class_head_teachers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			class_id INTEGER NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
			teacher_id INTEGER NOT NULL REFERENCES teachers(id) ON DELETE CASCADE,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id) ON DELETE CASCADE,
			phase TEXT NOT NULL CHECK(phase IN ('g1','g2','g3')),
			status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','ended')),
			started_at TEXT NOT NULL,
			ended_at TEXT,
			end_reason TEXT NOT NULL DEFAULT ''
		)`},
		{SQL: `CREATE UNIQUE INDEX IF NOT EXISTS idx_class_head_teachers_active
			ON class_head_teachers(class_id) WHERE status = 'active'`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_class_head_teachers_teacher
			ON class_head_teachers(teacher_id, status)`},
		{SQL: `CREATE TABLE IF NOT EXISTS teaching_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id) ON DELETE CASCADE,
			phase TEXT NOT NULL CHECK(phase IN ('g1','g2','g3')),
			subject TEXT NOT NULL,
			name TEXT NOT NULL,
			kind TEXT NOT NULL DEFAULT 'admin' CHECK(kind IN ('admin','walking')),
			admin_class_id INTEGER REFERENCES classes(id),
			status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','archived','graduated')),
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			UNIQUE(cohort_id, phase, subject, name)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS students (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id) ON DELETE CASCADE,
			student_no TEXT NOT NULL DEFAULT '',
			candidate_code TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			gender TEXT NOT NULL DEFAULT '',
			current_class_id INTEGER REFERENCES classes(id),
			current_track TEXT NOT NULL DEFAULT '' CHECK(current_track IN ('','physics','history','common')),
			elective_combo TEXT NOT NULL DEFAULT '',
			foreign_lang TEXT NOT NULL DEFAULT 'english',
			status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','suspended','transferred','transferred_out','borrowed','left_school','graduated')),
			entry_total_score REAL,
			previous_student_id INTEGER REFERENCES students(id),
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`},
		{SQL: `CREATE UNIQUE INDEX IF NOT EXISTS idx_students_cohort_no
			ON students(cohort_id, student_no) WHERE student_no <> ''`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_students_cohort_cand
			ON students(cohort_id, candidate_code)`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_students_cohort_class
			ON students(cohort_id, current_class_id)`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_students_previous
			ON students(previous_student_id)`},
		{SQL: `CREATE TABLE IF NOT EXISTS teaching_group_students (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			teaching_group_id INTEGER NOT NULL REFERENCES teaching_groups(id) ON DELETE CASCADE,
			student_id INTEGER NOT NULL REFERENCES students(id) ON DELETE CASCADE,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id) ON DELETE CASCADE,
			phase TEXT NOT NULL CHECK(phase IN ('g1','g2','g3')),
			subject TEXT NOT NULL,
			started_at TEXT NOT NULL,
			ended_at TEXT,
			UNIQUE(teaching_group_id, student_id)
		)`},
		{SQL: `CREATE UNIQUE INDEX IF NOT EXISTS idx_tgs_active_student_subject
			ON teaching_group_students(student_id, cohort_id, phase, subject) WHERE ended_at IS NULL`},
		{SQL: `CREATE TABLE IF NOT EXISTS teaching_assignments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			teacher_id INTEGER NOT NULL REFERENCES teachers(id) ON DELETE CASCADE,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id) ON DELETE CASCADE,
			phase TEXT NOT NULL CHECK(phase IN ('g1','g2','g3')),
			teaching_group_id INTEGER NOT NULL REFERENCES teaching_groups(id) ON DELETE CASCADE,
			role TEXT NOT NULL DEFAULT 'primary' CHECK(role IN ('primary','assistant')),
			status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','ended')),
			started_at TEXT NOT NULL,
			ended_at TEXT,
			end_reason TEXT NOT NULL DEFAULT '',
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0
		)`},
		{SQL: `CREATE UNIQUE INDEX IF NOT EXISTS idx_teaching_assignments_active
			ON teaching_assignments(teacher_id, teaching_group_id, role) WHERE status = 'active'`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_teaching_assignments_teacher_status
			ON teaching_assignments(teacher_id, status)`},
		{SQL: `CREATE TABLE IF NOT EXISTS student_entry_scores (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_id INTEGER NOT NULL REFERENCES students(id) ON DELETE CASCADE,
			subject TEXT NOT NULL,
			score REAL NOT NULL,
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(student_id, subject)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS cohort_baseline_overrides (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id) ON DELETE CASCADE,
			phase TEXT NOT NULL DEFAULT '',
			scope_type TEXT NOT NULL CHECK(scope_type IN ('class','grade')),
			scope_id INTEGER NOT NULL DEFAULT 0,
			subject TEXT NOT NULL,
			average REAL NOT NULL,
			count INTEGER NOT NULL DEFAULT 0,
			ratio REAL,
			source_name TEXT NOT NULL DEFAULT '',
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(cohort_id, phase, scope_type, scope_id, subject)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS student_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_id INTEGER NOT NULL REFERENCES students(id) ON DELETE CASCADE,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id) ON DELETE CASCADE,
			phase TEXT NOT NULL,
			event_type TEXT NOT NULL CHECK(event_type IN ('enrolled','promoted','class_changed','track_changed','status_changed','merged','graduated')),
			from_value_json TEXT NOT NULL DEFAULT '{}',
			to_value_json TEXT NOT NULL DEFAULT '{}',
			exam_id INTEGER,
			note TEXT NOT NULL DEFAULT '',
			occurred_at TEXT NOT NULL
		)`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_student_events_student
			ON student_events(student_id, occurred_at)`},

		// 2.3 考试、成绩快照与考核结果
		{SQL: `CREATE TABLE IF NOT EXISTS exams (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			academic_year_start INTEGER NOT NULL,
			term TEXT NOT NULL CHECK(term IN ('autumn','spring')),
			exam_type TEXT NOT NULL CHECK(exam_type IN ('monthly','midterm','final','mock','gaokao')),
			name TEXT NOT NULL,
			exam_date TEXT NOT NULL DEFAULT '',
			sequence INTEGER NOT NULL DEFAULT 1,
			status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','scored','assessed','archived')),
			created_at TEXT NOT NULL,
			UNIQUE(academic_year_start, term, exam_type, name)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS exam_cohorts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			exam_id INTEGER NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id) ON DELETE CASCADE,
			phase_at_exam TEXT NOT NULL CHECK(phase_at_exam IN ('g1','g2','g3','gaokao')),
			subjects_json TEXT NOT NULL DEFAULT '[]',
			UNIQUE(exam_id, cohort_id)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS exam_participants (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			exam_cohort_id INTEGER NOT NULL REFERENCES exam_cohorts(id) ON DELETE CASCADE,
			student_id INTEGER NOT NULL REFERENCES students(id) ON DELETE CASCADE,
			class_id INTEGER REFERENCES classes(id),
			class_name_snapshot TEXT NOT NULL,
			track_snapshot TEXT NOT NULL DEFAULT '',
			elective_snapshot TEXT NOT NULL DEFAULT '',
			foreign_lang_snapshot TEXT NOT NULL DEFAULT 'english',
			status_snapshot TEXT NOT NULL DEFAULT 'active',
			exam_no TEXT NOT NULL DEFAULT '',
			candidate_no TEXT NOT NULL DEFAULT '',
			total_score REAL,
			grade_rank INTEGER,
			grade_total_count INTEGER,
			track_rank INTEGER,
			track_total_count INTEGER,
			class_rank INTEGER,
			class_total_count INTEGER,
			is_valid_for_calc INTEGER NOT NULL DEFAULT 1,
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(exam_cohort_id, student_id)
		)`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_exam_participants_student_ec
			ON exam_participants(student_id, exam_cohort_id)`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_exam_participants_ec_class
			ON exam_participants(exam_cohort_id, class_id)`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_exam_participants_ec_rank
			ON exam_participants(exam_cohort_id, grade_rank)`},
		{SQL: `CREATE TABLE IF NOT EXISTS exam_scores (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			exam_participant_id INTEGER NOT NULL REFERENCES exam_participants(id) ON DELETE CASCADE,
			subject TEXT NOT NULL,
			score REAL,
			flag TEXT NOT NULL DEFAULT '',
			teaching_group_id_snapshot INTEGER REFERENCES teaching_groups(id),
			subject_grade_rank INTEGER,
			subject_class_rank INTEGER,
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(exam_participant_id, subject)
		)`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_exam_scores_subj_rank
			ON exam_scores(subject, subject_grade_rank)`},
		{SQL: `CREATE TABLE IF NOT EXISTS exam_cutoffs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			exam_cohort_id INTEGER NOT NULL REFERENCES exam_cohorts(id) ON DELETE CASCADE,
			kind TEXT NOT NULL CHECK(kind IN ('subject','tier')),
			"key" TEXT NOT NULL,
			track TEXT NOT NULL,
			cutoff REAL NOT NULL,
			UNIQUE(exam_cohort_id, kind, "key", track)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS assessments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			exam_cohort_id INTEGER NOT NULL UNIQUE REFERENCES exam_cohorts(id) ON DELETE CASCADE,
			plan_version_id INTEGER REFERENCES plan_versions(id),
			status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','calculated','stale')),
			current_run_id INTEGER,
			revision INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS assessment_class_targets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			assessment_id INTEGER NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
			class_id INTEGER NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
			tier_key TEXT NOT NULL,
			target_count REAL,
			actual_count REAL,
			UNIQUE(assessment_id, class_id, tier_key)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS assessment_stat_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			assessment_id INTEGER NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
			subject TEXT NOT NULL,
			name TEXT NOT NULL,
			lead_class_id INTEGER REFERENCES classes(id),
			target_count REAL,
			baseline_average REAL NOT NULL DEFAULT 0,
			baseline_ratio REAL NOT NULL DEFAULT 0,
			baseline_source TEXT NOT NULL DEFAULT '',
			explicit INTEGER NOT NULL DEFAULT 0,
			UNIQUE(assessment_id, subject, name)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS assessment_stat_group_members (
			stat_group_id INTEGER NOT NULL REFERENCES assessment_stat_groups(id) ON DELETE CASCADE,
			teaching_group_id INTEGER NOT NULL REFERENCES teaching_groups(id) ON DELETE CASCADE,
			PRIMARY KEY (stat_group_id, teaching_group_id)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS assessment_target_adjustments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			assessment_id INTEGER NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
			class_id INTEGER NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
			tier_key TEXT NOT NULL,
			adjusted_target REAL,
			excluded INTEGER NOT NULL DEFAULT 0,
			reason TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL,
			UNIQUE(assessment_id, class_id, tier_key)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS result_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			assessment_id INTEGER NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
			plan_version_id INTEGER NOT NULL REFERENCES plan_versions(id),
			input_hash TEXT NOT NULL,
			input_summary_json TEXT NOT NULL DEFAULT '',
			is_official INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS result_rows (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL REFERENCES result_runs(id) ON DELETE CASCADE,
			stat_group_id INTEGER NOT NULL,
			stat_group_name TEXT NOT NULL,
			subject TEXT NOT NULL,
			lead_class_id INTEGER,
			lead_class_name TEXT NOT NULL,
			class_count INTEGER NOT NULL,
			class_average REAL NOT NULL,
			grade_average REAL NOT NULL,
			baseline_average REAL NOT NULL,
			baseline_ratio REAL NOT NULL,
			improvement_raw REAL NOT NULL,
			improvement_score REAL NOT NULL,
			pass_count INTEGER NOT NULL,
			pass_target REAL NOT NULL,
			pass_raw REAL NOT NULL,
			pass_score REAL NOT NULL,
			completion_actual REAL NOT NULL,
			completion_base REAL NOT NULL,
			completion_raw REAL NOT NULL,
			completion_participate INTEGER NOT NULL,
			completion_score REAL NOT NULL,
			total_score REAL NOT NULL,
			rank INTEGER NOT NULL,
			tier_index INTEGER NOT NULL,
			points REAL NOT NULL,
			participants_json TEXT NOT NULL DEFAULT '[]',
			indicator_values_json TEXT NOT NULL DEFAULT '{}'
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS result_row_teachers (
			result_row_id INTEGER NOT NULL REFERENCES result_rows(id) ON DELETE CASCADE,
			teacher_id INTEGER NOT NULL REFERENCES teachers(id) ON DELETE CASCADE,
			teacher_name_snapshot TEXT NOT NULL,
			PRIMARY KEY (result_row_id, teacher_id)
		)`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_result_row_teachers_teacher
			ON result_row_teachers(teacher_id)`},
		{SQL: `CREATE TABLE IF NOT EXISTS personal_scores (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL REFERENCES result_runs(id) ON DELETE CASCADE,
			teacher_id INTEGER NOT NULL REFERENCES teachers(id) ON DELETE CASCADE,
			teacher_name_snapshot TEXT NOT NULL,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id) ON DELETE CASCADE,
			phase_at_exam TEXT NOT NULL,
			subject TEXT NOT NULL,
			group_points_json TEXT NOT NULL,
			score REAL NOT NULL,
			UNIQUE(run_id, teacher_id, subject)
		)`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_personal_scores_teacher
			ON personal_scores(teacher_id)`},
		{SQL: `CREATE TABLE IF NOT EXISTS export_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL REFERENCES result_runs(id) ON DELETE CASCADE,
			file_path TEXT NOT NULL,
			file_hash TEXT NOT NULL,
			exported_at TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS import_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			scope_type TEXT NOT NULL CHECK(scope_type IN ('school','cohort','exam')),
			scope_id INTEGER NOT NULL DEFAULT 0,
			kind TEXT NOT NULL,
			file_hash TEXT NOT NULL,
			relative_path TEXT NOT NULL,
			stored_path TEXT NOT NULL DEFAULT '',
			converted_path TEXT NOT NULL DEFAULT '',
			file_format TEXT NOT NULL DEFAULT '',
			row_count INTEGER NOT NULL DEFAULT 0,
			inserted_count INTEGER NOT NULL DEFAULT 0,
			updated_count INTEGER NOT NULL DEFAULT 0,
			skipped_count INTEGER NOT NULL DEFAULT 0,
			blocking_count INTEGER NOT NULL DEFAULT 0,
			warning_count INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'applied' CHECK(status IN ('applied','undone','failed')),
			mapping_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS import_sheets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			import_run_id INTEGER NOT NULL REFERENCES import_runs(id) ON DELETE CASCADE,
			sheet_name TEXT NOT NULL,
			role TEXT NOT NULL,
			header_row INTEGER NOT NULL DEFAULT 0,
			data_range TEXT NOT NULL DEFAULT '',
			version_tag TEXT NOT NULL DEFAULT '',
			is_current INTEGER NOT NULL DEFAULT 1,
			UNIQUE(import_run_id, sheet_name)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS import_columns (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			import_sheet_id INTEGER NOT NULL REFERENCES import_sheets(id) ON DELETE CASCADE,
			source_column TEXT NOT NULL,
			standard_field TEXT NOT NULL,
			UNIQUE(import_sheet_id, source_column)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS import_issues (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			import_run_id INTEGER NOT NULL REFERENCES import_runs(id) ON DELETE CASCADE,
			severity TEXT NOT NULL,
			row_num INTEGER NOT NULL,
			col_name TEXT NOT NULL,
			code TEXT NOT NULL,
			args_json TEXT NOT NULL DEFAULT '[]'
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS import_changes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			import_run_id INTEGER NOT NULL REFERENCES import_runs(id) ON DELETE CASCADE,
			table_name TEXT NOT NULL,
			row_id INTEGER NOT NULL,
			action TEXT NOT NULL,
			before_json TEXT NOT NULL DEFAULT '',
			after_json TEXT NOT NULL DEFAULT ''
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS data_revisions (
			scope_key TEXT PRIMARY KEY,
			revision INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id INTEGER NOT NULL DEFAULT 0,
			payload_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL
		)`},
	}
}
