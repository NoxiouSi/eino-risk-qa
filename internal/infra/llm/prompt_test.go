package llm_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
	"github.com/NoxiouSi/eino-risk-qa/internal/infra/llm"
)

func TestBuildMessages_IncludesAllSkillsAndRealImageContent(t *testing.T) {
	imageData := []byte("test-image-bytes")
	imagePath := filepath.Join(t.TempDir(), "evidence.png")
	require.NoError(t, os.WriteFile(imagePath, imageData, 0600))
	messages, err := llm.BuildMessages(riskfactor.JudgeInput{
		Questions: []riskfactor.QuestionSpec{{QuestionKey: "evidence", QuestionText: "证据图片", AnswerType: "image", Required: true, Rules: []string{"必须为对应证据", "不得存在P图痕迹"}}},
		Answers:   []riskfactor.QuestionAnswer{{QuestionKey: "evidence", ValueType: "image", ImagePaths: []string{imagePath}}},
	})

	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.Contains(t, messages[0].Content, "必须为对应证据")
	assert.Contains(t, messages[0].Content, "不得存在P图痕迹")
	require.Len(t, messages[1].UserInputMultiContent, 3)
	require.NotNil(t, messages[1].UserInputMultiContent[2].Image)
	require.NotNil(t, messages[1].UserInputMultiContent[2].Image.Base64Data)
	assert.Equal(t, base64.StdEncoding.EncodeToString(imageData), *messages[1].UserInputMultiContent[2].Image.Base64Data)
	assert.Equal(t, "image/png", messages[1].UserInputMultiContent[2].Image.MIMEType)
}
