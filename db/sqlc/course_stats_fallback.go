package db

import "context"

const checkChildModuleProgressExists = `-- name: CheckChildModuleProgressExists :one
SELECT EXISTS (
    SELECT 1
    FROM child_module_progress
    WHERE child_id = $1
);
`

func (q *Queries) CheckChildModuleProgressExists(ctx context.Context, childID string) (bool, error) {
	row := q.db.QueryRow(ctx, checkChildModuleProgressExists, childID)
	var exists bool
	err := row.Scan(&exists)
	return exists, err
}

const getPassedModuleIDsForCourse = `-- name: GetPassedModuleIDsForCourse :many
SELECT DISTINCT module_id
FROM child_quiz_attempts
WHERE child_id = $1
  AND course_id = $2
  AND module_id = ANY($3::text[])
  AND passed = true
  AND module_id <> '';
`

type GetPassedModuleIDsForCourseParams struct {
	ChildID   string   `json:"child_id"`
	CourseID  string   `json:"course_id"`
	ModuleIDs []string `json:"module_ids"`
}

func (q *Queries) GetPassedModuleIDsForCourse(ctx context.Context, arg GetPassedModuleIDsForCourseParams) ([]string, error) {
	rows, err := q.db.Query(ctx, getPassedModuleIDsForCourse, arg.ChildID, arg.CourseID, arg.ModuleIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []string{}
	for rows.Next() {
		var moduleID string
		if err := rows.Scan(&moduleID); err != nil {
			return nil, err
		}
		items = append(items, moduleID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
