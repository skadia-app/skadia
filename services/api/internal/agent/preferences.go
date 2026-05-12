package agent

type ChatPreferences struct {
	ChatWriteToolsEnabled     bool     `json:"chat_write_tools_enabled"`
	ChatAllowedWriteTools     []string `json:"chat_allowed_write_tools"`
	ChatIncludeExtractedData  bool     `json:"chat_include_extracted_data"`
	ChatIncludeHealthData     bool     `json:"chat_include_health_data"`
	ChatIncludeLocationData   bool     `json:"chat_include_location_data"`
	ChatIncludeRouteData      bool     `json:"chat_include_route_data"`
	ChatIncludeActivityData   bool     `json:"chat_include_activity_data"`
	ChatIncludeEquipmentData  bool     `json:"chat_include_equipment_data"`
	ChatIncludeUserNotes      bool     `json:"chat_include_user_notes"`
	ChatIncludeExternalRoutes bool     `json:"chat_include_external_routes"`
}

type PreferenceLoader struct{}
