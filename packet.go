package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Leaves room for Base64 encoding, HTML, and message headers.
const maxPacketFilesSize = 12_000_000

type packetFile struct {
	Name string `json:"name"`
	Data []byte `json:"content"`
}

func readPacketFiles(paths []string) ([]packetFile, [32]byte, error) {
	files := make([]packetFile, 0, len(paths))
	remaining := maxPacketFilesSize
	for _, path := range paths {
		name := filepath.Base(path)
		ext := strings.ToLower(filepath.Ext(name))
		if !slices.Contains(strings.Fields(".pdf .doc .docx .xls .xlsx .csv .txt .odt .ods .ppt .pptx .png .jpg .jpeg .gif .zip .ics .rtf"), ext) {
			return nil, [32]byte{}, fmt.Errorf("unsupported attachment type: %s", name)
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, [32]byte{}, fmt.Errorf("could not open %s", name)
		}
		data, err := io.ReadAll(io.LimitReader(file, int64(remaining)+1))
		file.Close()
		if err != nil || len(data) == 0 {
			return nil, [32]byte{}, fmt.Errorf("could not read a non-empty file: %s", name)
		}
		if len(data) > remaining {
			return nil, [32]byte{}, fmt.Errorf("attachments must total 12 MB or less")
		}
		remaining -= len(data)
		files = append(files, packetFile{Name: name, Data: data})
	}
	encoded, err := json.Marshal(files)
	if err != nil {
		return nil, [32]byte{}, err
	}
	return files, sha256.Sum256(encoded), nil
}

type packetResult struct {
	accepted int
	total    int
	started  bool
}

type packetVersion struct {
	To []sender `json:"to"`
}

func (c brevoClient) sendPacket(ctx context.Context, p profile, d draft, content string, files []packetFile, test bool) (packetResult, error) {
	var result packetResult
	if err := d.validate(p); err != nil {
		return result, err
	}
	if len(files) == 0 || content == "" {
		return result, fmt.Errorf("preview a meeting packet with attachments first")
	}
	versions := []packetVersion{{To: []sender{{Email: p.TestEmail}}}}
	if !test {
		contacts, err := c.listContacts(ctx, p.APIKey, d.ListID)
		if err != nil {
			return result, fmt.Errorf("load mailing list: %w", err)
		}
		listID, _ := strconv.ParseInt(d.ListID, 10, 64)
		versions = nil
		excluded := make(map[string]bool)
		for _, contact := range contacts {
			if contact.EmailBlacklisted || slices.Contains(contact.ListUnsubscribed, listID) {
				excluded[strings.ToLower(contact.Email)] = true
			}
		}
		for _, contact := range contacts {
			key := strings.ToLower(contact.Email)
			if excluded[key] || !contactEmail(contact.Email) {
				continue
			}
			excluded[key] = true
			versions = append(versions, packetVersion{To: []sender{{Name: contact.name(), Email: contact.Email}}})
		}
	}
	result.total = len(versions)
	if result.total == 0 {
		return result, fmt.Errorf("no subscribed email recipients on the selected list")
	}
	payload := struct {
		Sender  sender `json:"sender"`
		ReplyTo struct {
			Email string `json:"email"`
		} `json:"replyTo"`
		Subject     string          `json:"subject"`
		HTML        string          `json:"htmlContent"`
		Attachments []packetFile    `json:"attachment"`
		Versions    []packetVersion `json:"messageVersions"`
		Tags        []string        `json:"tags"`
	}{Sender: p.Sender, Subject: d.Subject, HTML: content, Attachments: files, Tags: []string{"club-meeting-packet"}}
	payload.ReplyTo.Email = p.ReplyTo
	for start := 0; start < len(versions); start += 50 {
		batch := versions[start:min(start+50, len(versions))]
		payload.Versions = batch
		var response struct {
			MessageID  string   `json:"messageId"`
			MessageIDs []string `json:"messageIds"`
		}
		result.started = true
		if err := c.request(ctx, p.APIKey, http.MethodPost, "smtp/email", payload, &response); err != nil {
			return result, err
		}
		confirmed := len(response.MessageIDs)
		if confirmed == 0 && response.MessageID != "" {
			confirmed = 1
		}
		if confirmed != len(batch) {
			return result, fmt.Errorf("Brevo returned an unexpected number of message IDs")
		}
		result.accepted += confirmed
	}
	return result, nil
}
