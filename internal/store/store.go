// Package store — in-memory хранилище настроек проектов, маппингов и согласий.
// В проде это MySQL; здесь данные грузятся из JSON-файлов.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Project — настройки интеграции проекта (игры) с MMP.
type Project struct {
	ID                  int    `json:"id"`
	Name                string `json:"name"`
	AppID               string `json:"app_id"`
	Enabled             bool   `json:"enabled"`
	ConsentCheckEnabled *bool  `json:"consent_check_enabled"`
}

// Mapping связывает пользователя (CUID) с его идентификатором в MMP для конкретного приложения.
type Mapping struct {
	AppID  string `json:"app_id"`
	UserID string `json:"user_id"`
	MMPID  string `json:"mmp_id"`
}

// Consent — согласие пользователя на передачу данных в MMP (GDPR).
type Consent struct {
	ProjectID int    `json:"project_id"`
	UserID    string `json:"user_id"`
	Approved  bool   `json:"approved"`
}

type mappingKey struct{ appID, userID string }

type consentKey struct {
	projectID int
	userID    string
}

// Store хранит данные в памяти. После создания только читается.
type Store struct {
	projects map[int]Project
	mappings map[mappingKey]string
	consents map[consentKey]Consent
}

// New собирает хранилище из готовых срезов.
func New(projects []Project, mappings []Mapping, consents []Consent) *Store {
	s := &Store{
		projects: make(map[int]Project, len(projects)),
		mappings: make(map[mappingKey]string, len(mappings)),
		consents: make(map[consentKey]Consent, len(consents)),
	}
	for _, p := range projects {
		s.projects[p.ID] = p
	}
	for _, m := range mappings {
		s.mappings[mappingKey{m.AppID, m.UserID}] = m.MMPID
	}
	for _, c := range consents {
		s.consents[consentKey{c.ProjectID, c.UserID}] = c
	}
	return s
}

// Load читает projects.json, mappings.json и consents.json из каталога dir.
func Load(dir string) (*Store, error) {
	var (
		projects []Project
		mappings []Mapping
		consents []Consent
	)
	files := []struct {
		name string
		dst  any
	}{
		{"projects.json", &projects},
		{"mappings.json", &mappings},
		{"consents.json", &consents},
	}
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(dir, f.name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.name, err)
		}
		if err := json.Unmarshal(raw, f.dst); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f.name, err)
		}
	}
	return New(projects, mappings, consents), nil
}

// Project возвращает настройки проекта.
func (s *Store) Project(id int) (Project, bool) {
	p, ok := s.projects[id]
	return p, ok
}

// MMPID возвращает идентификатор пользователя в MMP для приложения.
func (s *Store) MMPID(appID, userID string) (string, bool) {
	id, ok := s.mappings[mappingKey{appID, userID}]
	return id, ok
}

// Consent возвращает запись о согласии пользователя в проекте.
func (s *Store) Consent(projectID int, userID string) (Consent, bool) {
	c, ok := s.consents[consentKey{projectID, userID}]
	return c, ok
}
