package auth

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

type credentials struct {
	AccessToken string `yaml:"access_token"`
}

// GetSavedAccessToken reads the access token stored in ~/.fly/credentials.yml
func GetSavedAccessToken() (string, error) {
	credentials, err := loadCredentials()
	if err != nil {
		return "", err
	}

	return credentials.AccessToken, nil
}

// SetSavedAccessToken stores the provided access token in ~/.fly/credentials.yml
func SetSavedAccessToken(token string) error {
	credentials, err := loadCredentials()
	if err != nil {
		return err
	}

	credentials.AccessToken = token

	return saveCredentials(credentials)
}

// ClearSavedAccessToken removes the stored credentials in ~/.fly/credentials.yml
func ClearSavedAccessToken() error {
	credentialsPath, err := credentialsPath()
	if err != nil {
		return err
	}

	return os.Remove(credentialsPath)
}

func loadCredentials() (credentials, error) {
	var credentials credentials

	credentialsPath, err := credentialsPath()
	if err != nil {
		return credentials, err
	}

	data, err := ioutil.ReadFile(credentialsPath)
	if err != nil {
		return credentials, err
	}

	err = yaml.Unmarshal([]byte(data), &credentials)
	if err != nil {
		return credentials, err
	}

	return credentials, nil
}

func saveCredentials(credentials credentials) error {
	credentialsPath, err := credentialsPath()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(&credentials)
	log.Println(data)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(credentialsPath, data, 0644)
}

func credentialsPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(homeDir, ".fly", "credentials.yml"), nil
}
