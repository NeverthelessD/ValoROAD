package main

// 로그인 열쇠(라이엇 쿠키) 보관
//
// 맥: 키체인(서비스 이름 "ValoROAD", 계정 이름 = 계정 id). /usr/bin/security 로 넣고 꺼내요.
//   - 넣을 때는 열쇠가 프로그램 목록(ps)에 보이지 않도록 `security -i` 에 표준 입력으로 명령을 보내요.
//   - security 도구가 만든 항목이라, 앱을 업데이트해도(서명이 바뀌어도) 키체인 암호를 다시 묻지 않아요.
// 리눅스(시험용) 또는 VALOROAD_SECRETS=file: Support/secrets/<id> 파일(0600).

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const keychainService = "ValoROAD"

type SecretStore interface {
	Get(id string) (string, error)
	Set(id, value string) error
	Delete(id string) error
	Name() string
}

var safeID = regexp.MustCompile(`^[a-z0-9]{6,40}$`)

func newSecretStore(support string) SecretStore {
	if runtime.GOOS == "darwin" && env("SECRETS") != "file" {
		return keychainStore{}
	}
	return fileStore{dir: filepath.Join(support, "secrets")}
}

// ---------------------------------------------------------------- 키체인

type keychainStore struct{}

func (keychainStore) Name() string { return "키체인" }

func (keychainStore) Get(id string) (string, error) {
	if !safeID.MatchString(id) {
		return "", errors.New("잘못된 계정 id")
	}
	out, err := exec.Command("/usr/bin/security", "find-generic-password", "-s", keychainService, "-a", id, "-w").Output()
	if err != nil {
		return "", errors.New("키체인에서 로그인 정보를 찾지 못했어요")
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		return "", errors.New("키체인의 로그인 정보를 읽지 못했어요")
	}
	return string(b), nil
}

func (k keychainStore) Set(id, value string) error {
	if !safeID.MatchString(id) {
		return errors.New("잘못된 계정 id")
	}
	enc := base64.StdEncoding.EncodeToString([]byte(value))
	cmd := exec.Command("/usr/bin/security", "-i")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -s %s -a %s -l %s -w %s\n", keychainService, id, keychainService, enc))
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	runErr := cmd.Run()
	if got, err := k.Get(id); err == nil && got == value {
		return nil
	}
	return fmt.Errorf("키체인에 로그인 정보를 저장하지 못했어요 (%v %s)", runErr, lastLine(out.String()))
}

func (keychainStore) Delete(id string) error {
	if !safeID.MatchString(id) {
		return errors.New("잘못된 계정 id")
	}
	exec.Command("/usr/bin/security", "delete-generic-password", "-s", keychainService, "-a", id).Run()
	return nil
}

// ---------------------------------------------------------------- 파일 (시험용)

type fileStore struct{ dir string }

func (fileStore) Name() string { return "파일(시험용)" }

func (f fileStore) Get(id string) (string, error) {
	if !safeID.MatchString(id) {
		return "", errors.New("잘못된 계정 id")
	}
	b, err := os.ReadFile(filepath.Join(f.dir, id))
	if err != nil {
		return "", errors.New("로그인 정보를 찾지 못했어요")
	}
	return string(b), nil
}

func (f fileStore) Set(id, value string) error {
	if !safeID.MatchString(id) {
		return errors.New("잘못된 계정 id")
	}
	os.MkdirAll(f.dir, 0o700)
	return os.WriteFile(filepath.Join(f.dir, id), []byte(value), 0o600)
}

func (f fileStore) Delete(id string) error {
	if !safeID.MatchString(id) {
		return errors.New("잘못된 계정 id")
	}
	os.Remove(filepath.Join(f.dir, id))
	return nil
}
