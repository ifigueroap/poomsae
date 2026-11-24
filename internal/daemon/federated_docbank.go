package daemon

import (
	"context"
	"net/http"
	"path"
	"slices"
	"strings"

	"github.com/ifigueroap/poomsae/internal/api"
	"github.com/ifigueroap/poomsae/internal/docbank"
)

// localAttachmentOrigin reports whether this daemon can read the attachment
// from its own Docbank. Former instance identities still count as local.
func localAttachmentOrigin(ctx context.Context, cfg ServerConfig, _ int64, origin string) (bool, error) {
	if origin == "" || origin == cfg.DB.InstanceUID() {
		return true, nil
	}
	former, err := cfg.DB.FormerInstanceUIDs(ctx)
	if err != nil {
		return false, internalAPIError(err)
	}
	return slices.Contains(former, origin), nil
}

func exposedDocbankNode(cfg ServerConfig, node docbank.Node) error {
	if node.TrashedAt != "" {
		return attachmentError(410, "attachment_content_missing")
	}
	folder := path.Clean("/" + strings.Trim(cfg.DocbankFolder, "/"))
	if node.Path == "" || !strings.HasPrefix(node.Path, "/") || path.Clean(node.Path) != node.Path || strings.ContainsAny(node.Path, "\\\x00") {
		return attachmentError(502, "docbank_scope_unavailable")
	}
	if folder != "/" && node.Path != folder && !strings.HasPrefix(node.Path, folder+"/") {
		return api.NewError(http.StatusForbidden, "docbank_outside_scope", "file is outside the configured Docbank folder", "move the file into the configured folder", nil)
	}
	return nil
}
