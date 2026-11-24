package daemon

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/ifigueroap/poomsae/internal/api"
	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/docbank"
	"github.com/ifigueroap/poomsae/internal/metadata"
	"github.com/ifigueroap/poomsae/internal/shellquote"
)

var docbankLinkErrors = map[int]error{
	401: attachmentError(502, "docbank_unauthorized"),
	403: attachmentError(502, "docbank_unauthorized"),
	404: api.NewError(404, "docbank_not_found", "Docbank file or version not found", "", nil),
}

func linkAttachment(ctx context.Context, cfg ServerConfig, in *api.LinkAttachmentRequest) (*api.AddAttachmentResponse, error) {
	actor, err := attributedActor(ctx, in.Body.Actor)
	if err != nil {
		return nil, err
	}
	iss, err := activeIssueByRef(ctx, cfg.DB, in.ProjectID, in.Ref, db.IncludeDeletedNo)
	if err != nil {
		return nil, err
	}
	if err := requireFederatedIssueClaim(ctx, cfg, iss.ProjectID, iss, actor); err != nil {
		return nil, err
	}
	input := in.Body
	attachment, err := resolveDocbankAttachment(ctx, cfg, input, false)
	if err != nil {
		return nil, err
	}
	if input.ReuseExisting {
		return saveAttachment(ctx, cfg, in.ProjectID, iss, actor, attachment, &attachment.DocbankVersion)
	}
	return saveAttachment(ctx, cfg, in.ProjectID, iss, actor, attachment, nil)
}

func attachmentForVersion(iss db.Issue, version docbank.Version, origin string, localOrigins []string) (string, metadata.Attachment, error) {
	var values map[string]jsontext.Value
	if err := json.Unmarshal([]byte(iss.Metadata), &values); err != nil {
		return "", metadata.Attachment{}, err
	}
	var bestID string
	var best metadata.Attachment
	for key, raw := range values {
		id, ok := strings.CutPrefix(key, metadata.AttachmentPrefix)
		if !ok || id == "" {
			continue
		}
		attachment, valid := metadata.Valid(raw)
		if !valid {
			message := fmt.Sprintf("cannot read %q; remove it with poomsae docbank attachments rm %s %s", key, shellquote.Quote(iss.ShortID), shellquote.Quote(id))
			return "", metadata.Attachment{}, api.NewError(400, "attachment_invalid", message, "", nil)
		}
		if attachment.DocbankVersion != version.ID {
			continue
		}
		// A shared reference must name exactly the origin its readers expect.
		// Only an unmarked local link may reuse a reference this instance made
		// under a former identity.
		sameOrigin := attachment.Origin == origin || (origin == "" && slices.Contains(localOrigins, attachment.Origin))
		if !sameOrigin {
			continue
		}
		if bestID == "" || id < bestID {
			bestID, best = id, attachment
		}
	}
	return bestID, best, nil
}

func attachmentMediaType(name string) string {
	if typ := attachmentTypes[strings.ToLower(path.Ext(name))]; typ != "" {
		return typ
	}
	return "application/octet-stream"
}

func resolveDocbankAttachment(ctx context.Context, cfg ServerConfig, input api.LinkAttachmentInput, scoped bool) (metadata.Attachment, error) {
	if cfg.Docbank == nil {
		return metadata.Attachment{}, attachmentError(503, "docbank_not_configured")
	}
	var err error
	if (input.NodeID == nil) == (input.Path == nil) || (input.NodeID != nil && *input.NodeID <= 0) {
		return metadata.Attachment{}, attachmentError(400, "attachment_invalid")
	}
	var node docbank.Node
	if input.Path != nil {
		node, err = cfg.Docbank.ResolvePath(ctx, *input.Path)
	} else {
		node, err = cfg.Docbank.Node(ctx, *input.NodeID)
	}
	if err != nil {
		return metadata.Attachment{}, docbankError(err, docbankLinkErrors)
	}
	if scoped {
		if err := exposedDocbankNode(cfg, node); err != nil {
			return metadata.Attachment{}, err
		}
	} else if node.TrashedAt != "" {
		return metadata.Attachment{}, attachmentError(410, "attachment_content_missing")
	}
	if node.Kind != "file" {
		return metadata.Attachment{}, attachmentError(400, "attachment_not_file")
	}
	version := docbank.Version{ID: node.CurrentVersionID, NodeID: node.ID, Size: node.Size}
	if input.VersionID != nil {
		version, err = cfg.Docbank.Version(ctx, *input.VersionID)
		if err != nil {
			return metadata.Attachment{}, docbankError(err, docbankLinkErrors)
		}
	}
	if version.NodeID != node.ID {
		return metadata.Attachment{}, attachmentError(400, "attachment_version_mismatch")
	}
	if !metadata.ValidAttachmentName(node.Name) {
		return metadata.Attachment{}, attachmentError(400, "attachment_invalid")
	}
	return metadata.Attachment{Name: node.Name, MediaType: attachmentMediaType(node.Name), Size: version.Size, DocbankVersion: version.ID}, nil
}
