package agent

import "context"

type ChatPreferences struct {
	ChatWriteToolsEnabled           bool     `json:"chat_write_tools_enabled"`
	ChatAllowedWriteTools           []string `json:"chat_allowed_write_tools"`
	ChatExposeDocumentExtractedData bool     `json:"chat_expose_document_extracted_data"`
	ChatIncludeExtractedData        bool     `json:"chat_include_extracted_data"`
	ChatIncludeHealthData           bool     `json:"chat_include_health_data"`
	ChatIncludeLocationData         bool     `json:"chat_include_location_data"`
	ChatIncludeRouteData            bool     `json:"chat_include_route_data"`
	ChatIncludeActivityData         bool     `json:"chat_include_activity_data"`
	ChatIncludeEquipmentData        bool     `json:"chat_include_equipment_data"`
	ChatIncludeUserNotes            bool     `json:"chat_include_user_notes"`
	ChatIncludeExternalRoutes       bool     `json:"chat_include_external_routes"`
}

type PreferenceLoader struct {
	Defaults ChatPreferences
	LoadFunc func(ctx context.Context, userID string) (*ChatPreferences, error)
}

func (l *PreferenceLoader) LoadChatPreferences(ctx context.Context, userID string) (*ChatPreferences, error) {
	if l == nil {
		return &ChatPreferences{}, nil
	}
	if l.LoadFunc != nil {
		return l.LoadFunc(ctx, userID)
	}
	prefs := l.Defaults
	return &prefs, nil
}
