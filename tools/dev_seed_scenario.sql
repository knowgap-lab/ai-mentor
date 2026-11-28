-- Синтетические строки выпуска 1 для уже существующих экранов.
-- Повторный запуск не добавляет и не переписывает строки.
-- __CATALOG_VERSION__ подставляет tools/dev_seed.py целым числом.
-- Ключей ответов здесь нет. Имена выдуманы.

BEGIN;

INSERT INTO core.student_pii (student_id, display_name, telegram_id, birth_date)
SELECT 'student-demo', 'Демо Ученик', 'tg-demo-student', DATE '2010-03-14'
WHERE NOT EXISTS (SELECT 1 FROM core.student_pii WHERE student_id = 'student-demo');

INSERT INTO core.student_pseudonym (student_id, code)
SELECT 'student-demo', 'demo-k7'
WHERE NOT EXISTS (SELECT 1 FROM core.student_pseudonym WHERE student_id = 'student-demo');

INSERT INTO core.student_profile (student_id, subjects, exam_kind, exam_date)
SELECT 'student-demo', '["math"]'::jsonb, 'ct', DATE '2027-06-15'
WHERE NOT EXISTS (SELECT 1 FROM core.student_profile WHERE student_id = 'student-demo');

INSERT INTO core.consent (
    student_id, version, state, granted_at, revoked_at, anonymise_at, granted_by, method
)
SELECT
    'student-demo',
    1,
    'revoked',
    TIMESTAMPTZ '2026-02-01T10:00:00Z',
    TIMESTAMPTZ '2026-02-20T10:00:00Z',
    TIMESTAMPTZ '2026-03-22T10:00:00Z',
    'parent',
    'link_button'
WHERE NOT EXISTS (SELECT 1 FROM core.consent WHERE student_id = 'student-demo');

INSERT INTO core.study_plan (student_id, subject, exam_at, catalog_version)
SELECT 'student-demo', 'math', TIMESTAMPTZ '2027-06-15T09:00:00Z', __CATALOG_VERSION__
WHERE NOT EXISTS (
    SELECT 1 FROM core.study_plan
    WHERE student_id = 'student-demo' AND subject = 'math'
);

INSERT INTO core.schedule_rule (student_id, subject, weekdays, start_time, duration_min)
SELECT 'student-demo', 'math', ARRAY[1, 4]::integer[], '16:00', 60
WHERE NOT EXISTS (
    SELECT 1 FROM core.schedule_rule
    WHERE student_id = 'student-demo' AND subject = 'math'
);

INSERT INTO core.lesson (
    id, student_id, subject, place_no, title, kind, template_id, state,
    planned_week, starts_at, moved, added, no_blocks, issued, template, snapshot
)
SELECT
    'lesson-demo-held',
    'student-demo',
    'math',
    1,
    'Демо: сложение',
    'regular',
    'ct-demo:L01',
    'held',
    '2026-W10',
    TIMESTAMPTZ '2026-03-02T13:00:00Z',
    false,
    false,
    false,
    true,
    $template$[
      {"id":"ct-demo:L01.G1","title":"Новая тема","kind":"theory","minutes":[0,30],"blocks":["ct-demo:D1.B01","ct-demo:D1.B02"]},
      {"id":"ct-demo:L01.G2","title":"Практика","kind":"practice","minutes":[30,60],"practice":[{"stage_id":"ct-demo:L01.G2","variant_id":"ct-demo:D1.T01.S1.V001","type_id":"ct-demo:D1.T01","role":"solve_together","position":1,"theory_block":"ct-demo:D1.B01"}]}
    ]$template$::jsonb,
    $template$[
      {"id":"ct-demo:L01.G1","title":"Новая тема","kind":"theory","minutes":[0,30],"blocks":["ct-demo:D1.B01","ct-demo:D1.B02"]},
      {"id":"ct-demo:L01.G2","title":"Практика","kind":"practice","minutes":[30,60],"practice":[{"stage_id":"ct-demo:L01.G2","variant_id":"ct-demo:D1.T01.S1.V001","type_id":"ct-demo:D1.T01","role":"solve_together","position":1,"theory_block":"ct-demo:D1.B01"}]}
    ]$template$::jsonb
WHERE NOT EXISTS (SELECT 1 FROM core.lesson WHERE id = 'lesson-demo-held');

INSERT INTO core.lesson (
    id, student_id, subject, place_no, title, kind, template_id, state,
    planned_week, starts_at, moved, added, no_blocks, issued, template
)
SELECT
    'lesson-demo-next',
    'student-demo',
    'math',
    2,
    'Демо: следующий урок',
    'regular',
    'ct-demo:L01',
    'planned',
    '2026-W11',
    NULL,
    false,
    false,
    false,
    false,
    $template$[
      {"id":"ct-demo:L01.G1","title":"Новая тема","kind":"theory","minutes":[0,30],"blocks":["ct-demo:D1.B01"]},
      {"id":"ct-demo:L01.G2","title":"Практика","kind":"practice","minutes":[30,60],"practice":[]}
    ]$template$::jsonb
WHERE NOT EXISTS (SELECT 1 FROM core.lesson WHERE id = 'lesson-demo-next');

INSERT INTO core.lesson_mark (
    lesson_id, last_theory_block, practice_stage_id, practice_position, coverage
)
SELECT 'lesson-demo-held', 'ct-demo:D1.B01', 'ct-demo:L01.G2', 1, 'all'
WHERE NOT EXISTS (SELECT 1 FROM core.lesson_mark WHERE lesson_id = 'lesson-demo-held');

INSERT INTO core.lesson_topic (lesson_id, position, topic)
SELECT 'lesson-demo-held', 1, 'ct-demo:D1'
WHERE NOT EXISTS (
    SELECT 1 FROM core.lesson_topic
    WHERE lesson_id = 'lesson-demo-held' AND position = 1
);

INSERT INTO core.plan_snapshot (student_id, subject, version, source_hashes)
SELECT 'student-demo', 'math', 0, '{"ct-demo:L01":"dev-seed"}'::jsonb
WHERE NOT EXISTS (
    SELECT 1 FROM core.plan_snapshot
    WHERE student_id = 'student-demo' AND subject = 'math' AND version = 0
);

INSERT INTO core.plan_snapshot_lesson (
    student_id, subject, version, place_no, lesson_id, template_id, planned_week
)
SELECT 'student-demo', 'math', 0, 1, 'lesson-demo-held', 'ct-demo:L01', '2026-W10'
WHERE NOT EXISTS (
    SELECT 1 FROM core.plan_snapshot_lesson
    WHERE student_id = 'student-demo' AND subject = 'math' AND version = 0 AND place_no = 1
);

INSERT INTO core.plan_snapshot_lesson (
    student_id, subject, version, place_no, lesson_id, template_id, planned_week
)
SELECT 'student-demo', 'math', 0, 2, 'lesson-demo-next', 'ct-demo:L01', '2026-W11'
WHERE NOT EXISTS (
    SELECT 1 FROM core.plan_snapshot_lesson
    WHERE student_id = 'student-demo' AND subject = 'math' AND version = 0 AND place_no = 2
);

INSERT INTO core.variant_pool (student_id, variant_id, type_id, seen, reserve)
SELECT 'student-demo', 'ct-demo:D1.T01.S1.V001', 'ct-demo:D1.T01', true, false
WHERE NOT EXISTS (
    SELECT 1 FROM core.variant_pool
    WHERE student_id = 'student-demo' AND variant_id = 'ct-demo:D1.T01.S1.V001'
);

INSERT INTO core.variant_pool (student_id, variant_id, type_id, seen, reserve)
SELECT 'student-demo', 'ct-demo:D1.T01.S1.V002', 'ct-demo:D1.T01', true, false
WHERE NOT EXISTS (
    SELECT 1 FROM core.variant_pool
    WHERE student_id = 'student-demo' AND variant_id = 'ct-demo:D1.T01.S1.V002'
);

INSERT INTO core.variant_pool (student_id, variant_id, type_id, seen, reserve)
SELECT 'student-demo', 'ct-demo:D1.T01.S1.V003', 'ct-demo:D1.T01', false, false
WHERE NOT EXISTS (
    SELECT 1 FROM core.variant_pool
    WHERE student_id = 'student-demo' AND variant_id = 'ct-demo:D1.T01.S1.V003'
);

INSERT INTO core.homework_draft (id, lesson_id, items, deadline)
SELECT
    'draft-demo',
    'lesson-demo-next',
    '["ct-demo:D1.T01.S1.V003","ct-demo:D1.T01.S1.V004"]'::jsonb,
    '2026-03-15T18:00:00Z'
WHERE NOT EXISTS (SELECT 1 FROM core.homework_draft WHERE id = 'draft-demo');

INSERT INTO core.assignment (
    id, student_id, state, kind, catalog_version, deadline, place, snapshot
)
SELECT
    'asg-demo-submitted',
    'student-demo',
    'submitted',
    'homework',
    __CATALOG_VERSION__,
    TIMESTAMPTZ '2026-03-08T18:00:00Z',
    'home',
    $snap${
      "items": [
        {"item_id":"item-demo-ok","statement":"Найдите значение выражения $2 + 3$.","answer_format":"ct.integer","max_points":1},
        {"item_id":"item-demo-bad","statement":"Найдите значение выражения $3 + 4$.","answer_format":"ct.integer","max_points":1}
      ]
    }$snap$::jsonb
WHERE NOT EXISTS (SELECT 1 FROM core.assignment WHERE id = 'asg-demo-submitted');

INSERT INTO core.assignment_item (assignment_id, item_id, state, skipped, max_points)
SELECT 'asg-demo-submitted', 'item-demo-ok', 'final', false, 1
WHERE NOT EXISTS (
    SELECT 1 FROM core.assignment_item
    WHERE assignment_id = 'asg-demo-submitted' AND item_id = 'item-demo-ok'
);

INSERT INTO core.assignment_item (assignment_id, item_id, state, skipped, max_points)
SELECT 'asg-demo-submitted', 'item-demo-bad', 'final', false, 1
WHERE NOT EXISTS (
    SELECT 1 FROM core.assignment_item
    WHERE assignment_id = 'asg-demo-submitted' AND item_id = 'item-demo-bad'
);

INSERT INTO core.attempts (
    id, assignment_id, item_id, raw_input, journal_seq, source, place, attempt_no
)
SELECT
    'attempt-demo-ok', 'asg-demo-submitted', 'item-demo-ok', '5', 101,
    'homework', 'home', 1
WHERE NOT EXISTS (SELECT 1 FROM core.attempts WHERE id = 'attempt-demo-ok');

INSERT INTO core.attempts (
    id, assignment_id, item_id, raw_input, journal_seq, source, place, attempt_no
)
SELECT
    'attempt-demo-bad', 'asg-demo-submitted', 'item-demo-bad', '0', 102,
    'homework', 'home', 1
WHERE NOT EXISTS (SELECT 1 FROM core.attempts WHERE id = 'attempt-demo-bad');

INSERT INTO core.attempt_verdict (attempt_id, seq, verdict, points, reason)
SELECT 'attempt-demo-ok', 1, 'correct', 1, NULL
WHERE NOT EXISTS (
    SELECT 1 FROM core.attempt_verdict
    WHERE attempt_id = 'attempt-demo-ok' AND seq = 1
);

INSERT INTO core.attempt_verdict (attempt_id, seq, verdict, points, reason)
SELECT 'attempt-demo-bad', 1, 'wrong', 0, NULL
WHERE NOT EXISTS (
    SELECT 1 FROM core.attempt_verdict
    WHERE attempt_id = 'attempt-demo-bad' AND seq = 1
);

INSERT INTO core.observation (
    assignment_id, item_id, revision, first_attempt_points, total_points, max_points
)
SELECT 'asg-demo-submitted', 'item-demo-ok', 1, 1, 1, 1
WHERE NOT EXISTS (
    SELECT 1 FROM core.observation
    WHERE assignment_id = 'asg-demo-submitted' AND item_id = 'item-demo-ok' AND revision = 1
);

INSERT INTO core.observation (
    assignment_id, item_id, revision, first_attempt_points, total_points, max_points
)
SELECT 'asg-demo-submitted', 'item-demo-bad', 1, 0, 0, 1
WHERE NOT EXISTS (
    SELECT 1 FROM core.observation
    WHERE assignment_id = 'asg-demo-submitted' AND item_id = 'item-demo-bad' AND revision = 1
);

INSERT INTO core.dispute (assignment_id, item_id, state, reason, body)
SELECT
    'asg-demo-submitted',
    'item-demo-bad',
    'open',
    'statement_error',
    'В условии опечатка.'
WHERE NOT EXISTS (
    SELECT 1 FROM core.dispute
    WHERE assignment_id = 'asg-demo-submitted' AND item_id = 'item-demo-bad'
);

INSERT INTO core.recheck_batch (
    correction_id, state, total, processed, changed, skipped_manual,
    variants, started_at, finished_at
)
SELECT
    1,
    'done',
    1,
    1,
    1,
    0,
    ARRAY['ct-demo:D1.T01.S1.V002']::text[],
    TIMESTAMPTZ '2026-03-03T09:00:00Z',
    TIMESTAMPTZ '2026-03-03T09:05:00Z'
WHERE NOT EXISTS (SELECT 1 FROM core.recheck_batch WHERE correction_id = 1);

INSERT INTO core.journal_event (journal_seq, kind, payload, at, student_id, actor_role, actor_id)
SELECT
    1,
    'lesson_marked',
    '{"lesson_id":"lesson-demo-held","last_theory_block":"ct-demo:D1.B01","last_practice":{"stage_id":"ct-demo:L01.G2","position":1},"coverage":"all"}'::jsonb,
    TIMESTAMPTZ '2026-03-02T14:00:00Z',
    'student-demo',
    'tutor',
    'tutor-demo'
WHERE NOT EXISTS (SELECT 1 FROM core.journal_event WHERE journal_seq = 1);

INSERT INTO core.journal_event (journal_seq, kind, payload, at, student_id, actor_role, actor_id)
SELECT
    2,
    'assignment_issued',
    jsonb_build_object(
        'assignment_id', 'asg-demo-submitted',
        'kind', 'homework',
        'subject', 'math',
        'catalog_version', __CATALOG_VERSION__,
        'rules_version', 'demo-1',
        'deadline', '2026-03-08T18:00:00Z',
        'place', 'home',
        'lesson_id', 'lesson-demo-held',
        'items', jsonb_build_array(
            jsonb_build_object(
                'item_id', 'item-demo-ok',
                'variant_id', 'ct-demo:D1.T01.S1.V001',
                'answer_format', 'ct.integer',
                'max_points', 1,
                'source', 'new_topic'
            ),
            jsonb_build_object(
                'item_id', 'item-demo-bad',
                'variant_id', 'ct-demo:D1.T01.S1.V002',
                'answer_format', 'ct.integer',
                'max_points', 1,
                'source', 'new_topic'
            )
        )
    ),
    TIMESTAMPTZ '2026-03-02T14:05:00Z',
    'student-demo',
    'tutor',
    'tutor-demo'
WHERE NOT EXISTS (SELECT 1 FROM core.journal_event WHERE journal_seq = 2);

INSERT INTO core.journal_event (journal_seq, kind, payload, at, student_id, actor_role, actor_id)
SELECT
    3,
    'attempt_recorded',
    jsonb_build_object(
        'attempt_id', 'attempt-demo-ok',
        'assignment_id', 'asg-demo-submitted',
        'item_id', 'item-demo-ok',
        'variant_id', 'ct-demo:D1.T01.S1.V001',
        'catalog_version', __CATALOG_VERSION__,
        'raw_input', '5',
        'normalized_input', '5',
        'attempt_no', 1,
        'verdict', 'correct',
        'points', 1,
        'source', 'homework',
        'place', 'home',
        'rules_version', 'demo-1',
        'idempotency_key', 'demo-ok-1'
    ),
    TIMESTAMPTZ '2026-03-02T15:00:00Z',
    'student-demo',
    'student',
    'student-demo'
WHERE NOT EXISTS (SELECT 1 FROM core.journal_event WHERE journal_seq = 3);

INSERT INTO core.journal_event (journal_seq, kind, payload, at, student_id, actor_role, actor_id)
SELECT
    4,
    'attempt_verdict',
    '{"attempt_id":"attempt-demo-ok","origin":"scoring","verdict":"correct","points":1,"max_points":1}'::jsonb,
    TIMESTAMPTZ '2026-03-02T15:00:01Z',
    'student-demo',
    'system',
    NULL
WHERE NOT EXISTS (SELECT 1 FROM core.journal_event WHERE journal_seq = 4);

INSERT INTO core.journal_event (journal_seq, kind, payload, at, student_id, actor_role, actor_id)
SELECT
    5,
    'attempt_recorded',
    jsonb_build_object(
        'attempt_id', 'attempt-demo-bad',
        'assignment_id', 'asg-demo-submitted',
        'item_id', 'item-demo-bad',
        'variant_id', 'ct-demo:D1.T01.S1.V002',
        'catalog_version', __CATALOG_VERSION__,
        'raw_input', '0',
        'normalized_input', '0',
        'attempt_no', 1,
        'verdict', 'wrong',
        'points', 0,
        'source', 'homework',
        'place', 'home',
        'rules_version', 'demo-1',
        'idempotency_key', 'demo-bad-1'
    ),
    TIMESTAMPTZ '2026-03-02T15:02:00Z',
    'student-demo',
    'student',
    'student-demo'
WHERE NOT EXISTS (SELECT 1 FROM core.journal_event WHERE journal_seq = 5);

INSERT INTO core.journal_event (journal_seq, kind, payload, at, student_id, actor_role, actor_id)
SELECT
    6,
    'attempt_verdict',
    '{"attempt_id":"attempt-demo-bad","origin":"scoring","verdict":"wrong","points":0,"max_points":1}'::jsonb,
    TIMESTAMPTZ '2026-03-02T15:02:01Z',
    'student-demo',
    'system',
    NULL
WHERE NOT EXISTS (SELECT 1 FROM core.journal_event WHERE journal_seq = 6);

INSERT INTO core.journal_event (journal_seq, kind, payload, at, student_id, actor_role, actor_id)
SELECT
    7,
    'assignment_submitted',
    '{"assignment_id":"asg-demo-submitted","answered":2,"skipped":0}'::jsonb,
    TIMESTAMPTZ '2026-03-02T15:03:00Z',
    'student-demo',
    'student',
    'student-demo'
WHERE NOT EXISTS (SELECT 1 FROM core.journal_event WHERE journal_seq = 7);

INSERT INTO core.journal_event (journal_seq, kind, payload, at, student_id, actor_role, actor_id)
SELECT
    8,
    'dispute_opened',
    '{"assignment_id":"asg-demo-submitted","item_id":"item-demo-bad","reason":"statement_error","text":"В условии опечатка.","dispute_id":"dispute-demo-bad"}'::jsonb,
    TIMESTAMPTZ '2026-03-02T15:10:00Z',
    'student-demo',
    'student',
    'student-demo'
WHERE NOT EXISTS (SELECT 1 FROM core.journal_event WHERE journal_seq = 8);

INSERT INTO core.journal_event (journal_seq, kind, payload, at, student_id, actor_role, actor_id)
SELECT
    9,
    'consent_revoked',
    '{"consent_id":"consent-demo","anonymise_at":"2026-03-22T10:00:00Z"}'::jsonb,
    TIMESTAMPTZ '2026-02-20T10:00:00Z',
    'student-demo',
    'tutor',
    'tutor-demo'
WHERE NOT EXISTS (SELECT 1 FROM core.journal_event WHERE journal_seq = 9);

INSERT INTO core.journal_event (journal_seq, kind, payload, at, student_id, actor_role, actor_id)
SELECT
    10,
    'recheck_completed',
    '{"correction_id":1,"changed_items":["item-demo-bad"]}'::jsonb,
    TIMESTAMPTZ '2026-03-03T09:05:00Z',
    'student-demo',
    'system',
    NULL
WHERE NOT EXISTS (SELECT 1 FROM core.journal_event WHERE journal_seq = 10);

SELECT setval(
    'core.journal_seq',
    GREATEST(
        (SELECT COALESCE(MAX(journal_seq), 1) FROM core.journal_event),
        (SELECT COALESCE(MAX(journal_seq), 1) FROM core.attempts)
    )
);

COMMIT;
