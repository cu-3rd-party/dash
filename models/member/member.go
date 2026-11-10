// Package member is the list of Forgejo users allowed into the admin pages, with their roles.
package member

import (
	"context"
	"regexp"
	"strings"

	"git.cu3rd.ru/cu3rd/dash/models/db"
)

// Roles, from the least to the most privileged.
const (
	RoleViewer = "viewer" // sees everything, changes nothing
	RoleEditor = "editor" // expenses, payments, donations, categories
	RoleOwner  = "owner"  // also members and settings
)

// RoleInfo describes a role for forms.
type RoleInfo struct {
	Key, Label, Hint string
}

// Roles lists the roles for forms.
var Roles = []RoleInfo{
	{RoleViewer, "Наблюдатель", "видит всё, ничего не меняет"},
	{RoleEditor, "Редактор", "расходы, оплаты, донаты, категории"},
	{RoleOwner, "Владелец", "всё + пользователи и настройки"},
}

// RoleLevel orders roles; 0 is not a role.
func RoleLevel(role string) int {
	switch role {
	case RoleViewer:
		return 1
	case RoleEditor:
		return 2
	case RoleOwner:
		return 3
	}
	return 0
}

// RoleLabel is the Russian name of a role.
func RoleLabel(role string) string {
	for _, r := range Roles {
		if r.Key == role {
			return r.Label
		}
	}
	return role
}

// Member is a Forgejo user with access.
type Member struct {
	ID          int64  `xorm:"pk autoincr"`
	Login       string `xorm:"VARCHAR(40) NOT NULL UNIQUE"`
	Name        string `xorm:"VARCHAR(100) NOT NULL DEFAULT ''"`
	Role        string `xorm:"VARCHAR(16) NOT NULL"`
	Telegram    string `xorm:"VARCHAR(32) NOT NULL DEFAULT ''"` // empty: same as Login
	CreatedBy   string `xorm:"VARCHAR(40) NOT NULL DEFAULT ''"`
	CreatedUnix int64  `xorm:"created"`
}

func init() {
	db.RegisterModel(new(Member))
}

// Can reports whether the member has at least role.
func (m *Member) Can(role string) bool {
	return m != nil && RoleLevel(m.Role) >= RoleLevel(role)
}

// ErrMemberNotExist is returned for an unknown login.
type ErrMemberNotExist struct{ Login string }

func (err ErrMemberNotExist) Error() string {
	return "пользователь " + err.Login + " не найден"
}
func (err ErrMemberNotExist) Unwrap() error { return db.ErrNotExist }

// ErrInvalidMember is returned for bad input.
type ErrInvalidMember struct{ Reason string }

func (err ErrInvalidMember) Error() string { return err.Reason }
func (err ErrInvalidMember) Unwrap() error { return db.ErrInvalidArgument }

var loginRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,39}$`)

// NormalizeLogin lowercases a Forgejo login and checks its shape.
func NormalizeLogin(login string) (string, error) {
	login = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(login), "@"))
	if !loginRe.MatchString(login) {
		return "", ErrInvalidMember{Reason: "некорректный логин Forgejo"}
	}
	return login, nil
}

// GetByLogin returns a member.
func GetByLogin(ctx context.Context, login string) (*Member, error) {
	m := new(Member)
	has, err := db.GetEngine(ctx).Where("login = ?", login).Get(m)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrMemberNotExist{Login: login}
	}
	return m, nil
}

// List returns all members by login.
func List(ctx context.Context) ([]*Member, error) {
	members := make([]*Member, 0, 8)
	return members, db.GetEngine(ctx).Asc("login").Find(&members)
}

// CountOwners counts members with the owner role.
func CountOwners(ctx context.Context) (int64, error) {
	return db.GetEngine(ctx).Where("role = ?", RoleOwner).Count(new(Member))
}

// Upsert inserts m or updates the role (and the name, when given) of an existing member.
func Upsert(ctx context.Context, m *Member) error {
	if RoleLevel(m.Role) == 0 {
		return ErrInvalidMember{Reason: "неизвестная роль " + m.Role}
	}
	existing, err := GetByLogin(ctx, m.Login)
	if err == nil {
		cols := []string{"role"}
		existing.Role = m.Role
		if m.Name != "" {
			existing.Name = m.Name
			cols = append(cols, "name")
		}
		_, err = db.GetEngine(ctx).ID(existing.ID).Cols(cols...).Update(existing)
		return err
	}
	if !db.IsErrNotExist(err) {
		return err
	}
	_, err = db.GetEngine(ctx).Insert(m)
	return err
}

// Delete removes a member.
func Delete(ctx context.Context, login string) error {
	_, err := db.GetEngine(ctx).Where("login = ?", login).Delete(new(Member))
	return err
}

// TelegramHandle is the contact Telegram login: the chosen one, by default the Forgejo login.
func (m *Member) TelegramHandle() string {
	if m.Telegram != "" {
		return m.Telegram
	}
	return m.Login
}

var telegramRe = regexp.MustCompile(`^[A-Za-z0-9_]{5,32}$`)

// NormalizeTelegram accepts "name", "@name" or a t.me link; empty resets to the default.
func NormalizeTelegram(s string) (string, error) {
	s = strings.TrimSpace(s)
	for _, p := range []string{"https://", "http://", "t.me/", "@"} {
		s = strings.TrimPrefix(s, p)
	}
	if s != "" && !telegramRe.MatchString(s) {
		return "", ErrInvalidMember{Reason: "некорректный логин Telegram: 5–32 символа, латиница, цифры и _"}
	}
	return s, nil
}

// SetTelegram stores the contact Telegram login of a member (normalized, see NormalizeTelegram).
func SetTelegram(ctx context.Context, login, telegram string) error {
	_, err := db.GetEngine(ctx).Where("login = ?", login).Cols("telegram").Update(&Member{Telegram: telegram})
	return err
}

// Admins returns members who can edit (editors and owners), by login.
func Admins(ctx context.Context) ([]*Member, error) {
	members := make([]*Member, 0, 4)
	return members, db.GetEngine(ctx).In("role", RoleEditor, RoleOwner).Asc("login").Find(&members)
}
