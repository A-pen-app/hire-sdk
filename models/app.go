package models

type App struct {
	ID       string `json:"app_id" db:"id" example:"c718f5ca-724f-45c2-84fd-8e8a4fc77f10"`
	Name     string `json:"app_name" db:"name" example:"A-Pen"`
	BundleID string `json:"bundle_id" db:"bundle_id" example:"com.yoku.apen"`
}
