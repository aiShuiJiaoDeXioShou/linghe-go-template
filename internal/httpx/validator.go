package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// StructValidator 为 Fiber 请求绑定提供结构体校验
type StructValidator struct {
	validator *validator.Validate
}

// ValidationDetails 表示参数校验失败的字段列表
type ValidationDetails struct {
	Fields []FieldViolation `json:"fields"`
}

// FieldViolation 表示一个未通过校验的请求字段
type FieldViolation struct {
	Field     string `json:"field"`
	Rule      string `json:"rule"`
	Parameter string `json:"parameter,omitempty"`
	Message   string `json:"message"`
}

// NewStructValidator 创建使用请求字段名称的校验器
func NewStructValidator() *StructValidator {
	validate := validator.New(validator.WithRequiredStructEnabled())
	validate.RegisterTagNameFunc(requestFieldName)
	return &StructValidator{validator: validate}
}

// Validate 校验 Fiber 已绑定的请求结构体
func (v *StructValidator) Validate(out any) error {
	return v.validator.Struct(out)
}

// DecodeJSON 严格解析单个 JSON 值并拒绝未知字段
func DecodeJSON(data []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("请求体只能包含一个 JSON 值")
		}
		return err
	}
	return nil
}

func requestFieldName(field reflect.StructField) string {
	for _, key := range []string{"json", "query", "uri", "form", "header"} {
		name := strings.Split(field.Tag.Get(key), ",")[0]
		if name != "" && name != "-" {
			return name
		}
	}
	return field.Name
}

func newValidationDetails(validationErrors validator.ValidationErrors) ValidationDetails {
	fields := make([]FieldViolation, 0, len(validationErrors))
	for _, validationError := range validationErrors {
		field := validationField(validationError)
		fields = append(fields, FieldViolation{
			Field:     field,
			Rule:      validationError.Tag(),
			Parameter: validationError.Param(),
			Message:   validationMessage(field, validationError),
		})
	}
	return ValidationDetails{Fields: fields}
}

func validationField(fieldError validator.FieldError) string {
	namespace := fieldError.Namespace()
	if separator := strings.IndexByte(namespace, '.'); separator >= 0 {
		return namespace[separator+1:]
	}
	return fieldError.Field()
}

func validationMessage(field string, fieldError validator.FieldError) string {
	parameter := fieldError.Param()
	switch fieldError.Tag() {
	case "required":
		return field + "不能为空"
	case "email":
		return field + "必须是有效的邮箱地址"
	case "uuid", "uuid4":
		return field + "必须是有效的 UUID"
	case "url":
		return field + "必须是有效的 URL"
	case "oneof":
		return field + "必须是以下值之一 " + parameter
	case "len":
		return field + "长度必须为 " + parameter
	case "min":
		if isCollection(fieldError.Kind()) {
			return field + "长度不能小于 " + parameter
		}
		return field + "不能小于 " + parameter
	case "max":
		if isCollection(fieldError.Kind()) {
			return field + "长度不能大于 " + parameter
		}
		return field + "不能大于 " + parameter
	case "gt":
		return field + "必须大于 " + parameter
	case "gte":
		return field + "不能小于 " + parameter
	case "lt":
		return field + "必须小于 " + parameter
	case "lte":
		return field + "不能大于 " + parameter
	default:
		return field + "未通过 " + fieldError.Tag() + " 校验"
	}
}

func isCollection(kind reflect.Kind) bool {
	return kind == reflect.String || kind == reflect.Array || kind == reflect.Slice || kind == reflect.Map
}
