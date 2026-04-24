// services/api/internal/shadowfs/filetypes.go
package shadowfs

// UserFacingFileTypes is the canonical list of file types exposed in user-
// and admin-facing settings (spec §4.1). "report" and "chat" remain
// renderer-default-only and are NOT user-configurable.
//
// Referenced by:
//   - handler/shadowfs_settings.go (allowedFileTypes)
//   - handler/admin_library_settings.go (userFacingSchemaFileTypes)
//   - cmd/api/main.go (librarySeedDefaults)
var UserFacingFileTypes = []string{"activity", "media", "document", "health", "route"}
