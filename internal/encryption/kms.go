package encryption

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
)

type KMSClient struct {
	client *kms.Client
	keyID  string
}

func NewKMSClient() (*KMSClient, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %v", err)
	}

	keyID := os.Getenv("KMS_KEY_ID")
	if keyID == "" {
		return nil, fmt.Errorf("KMS_KEY_ID environment variable is required")
	}

	client := kms.NewFromConfig(cfg)
	return &KMSClient{
		client: client,
		keyID:  keyID,
	}, nil
}

// EncryptPHI encrypts PHI data using AWS KMS envelope encryption
func (k *KMSClient) EncryptPHI(ctx context.Context, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	input := &kms.EncryptInput{
		KeyId:     aws.String(k.keyID),
		Plaintext: []byte(plaintext),
		EncryptionContext: map[string]string{
			"Purpose": "PHI-Encryption",
			"Service": "Therma-Backend",
		},
	}

	result, err := k.client.Encrypt(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt PHI: %v", err)
	}

	// Return base64 encoded ciphertext
	return base64.StdEncoding.EncodeToString(result.CiphertextBlob), nil
}

// DecryptPHI decrypts PHI data using AWS KMS
func (k *KMSClient) DecryptPHI(ctx context.Context, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	// Decode base64 ciphertext
	ciphertextBlob, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %v", err)
	}

	input := &kms.DecryptInput{
		CiphertextBlob: ciphertextBlob,
		EncryptionContext: map[string]string{
			"Purpose": "PHI-Encryption",
			"Service": "Therma-Backend",
		},
	}

	result, err := k.client.Decrypt(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt PHI: %v", err)
	}

	return string(result.Plaintext), nil
}

// EncryptPHIArray encrypts an array of PHI strings
func (k *KMSClient) EncryptPHIArray(ctx context.Context, plaintexts []string) ([]string, error) {
	if len(plaintexts) == 0 {
		return []string{}, nil
	}

	encrypted := make([]string, len(plaintexts))
	for i, plaintext := range plaintexts {
		enc, err := k.EncryptPHI(ctx, plaintext)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt array element %d: %v", i, err)
		}
		encrypted[i] = enc
	}

	return encrypted, nil
}

// DecryptPHIArray decrypts an array of PHI strings
func (k *KMSClient) DecryptPHIArray(ctx context.Context, ciphertexts []string) ([]string, error) {
	if len(ciphertexts) == 0 {
		return []string{}, nil
	}

	decrypted := make([]string, len(ciphertexts))
	for i, ciphertext := range ciphertexts {
		dec, err := k.DecryptPHI(ctx, ciphertext)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt array element %d: %v", i, err)
		}
		decrypted[i] = dec
	}

	return decrypted, nil
}

// ValidateKMSKey validates that the KMS key exists and is accessible
func (k *KMSClient) ValidateKMSKey(ctx context.Context) error {
	input := &kms.DescribeKeyInput{
		KeyId: aws.String(k.keyID),
	}

	_, err := k.client.DescribeKey(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to validate KMS key %s: %v", k.keyID, err)
	}

	return nil
}

// GetKeyPolicy retrieves the key policy for audit purposes
func (k *KMSClient) GetKeyPolicy(ctx context.Context) (string, error) {
	input := &kms.GetKeyPolicyInput{
		KeyId:      aws.String(k.keyID),
		PolicyName: aws.String("default"),
	}

	result, err := k.client.GetKeyPolicy(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to get key policy: %v", err)
	}

	return *result.Policy, nil
}
