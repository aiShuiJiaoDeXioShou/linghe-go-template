package devtool

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var moduleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

type moduleOptions struct {
	domain string
	realm  string
	dryRun bool
}

type generatedFile struct {
	path    string
	content []byte
}

func (c command) newModule(args []string) error {
	options, err := parseModuleOptions(args)
	if err != nil {
		return err
	}
	projectModule, err := modulePath(c.root)
	if err != nil {
		return err
	}
	moduleDirectory := filepath.Join(c.root, "internal", "modules", options.domain)
	if _, err := os.Stat(moduleDirectory); err == nil {
		return fmt.Errorf("模块目录已存在: %s", relative(c.root, moduleDirectory))
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查模块目录: %w", err)
	}

	files, err := renderModuleFiles(moduleDirectory, projectModule, options)
	if err != nil {
		return err
	}
	modulesPath := filepath.Join(c.root, "internal", "app", "modules.go")
	modulesSource, err := os.ReadFile(modulesPath)
	if err != nil {
		return fmt.Errorf("读取 internal/app/modules.go: %w", err)
	}
	updatedModules, err := updateModulesSource(modulesSource, projectModule, options)
	if err != nil {
		return err
	}

	if options.dryRun {
		for _, file := range files {
			previewFile(c.stdout, c.root, file.path, file.content)
		}
		previewFile(c.stdout, c.root, modulesPath, updatedModules)
		return nil
	}
	if err := commitModule(moduleDirectory, files, modulesPath, updatedModules); err != nil {
		return err
	}
	for _, file := range files {
		_, _ = fmt.Fprintf(c.stdout, "created %s\n", relative(c.root, file.path))
	}
	_, _ = fmt.Fprintf(c.stdout, "updated %s\n", relative(c.root, modulesPath))
	return nil
}

func parseModuleOptions(args []string) (moduleOptions, error) {
	options := moduleOptions{realm: "none"}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch argument {
		case "--dry-run":
			options.dryRun = true
		case "--realm":
			index++
			if index >= len(args) {
				return moduleOptions{}, fmt.Errorf("--realm 缺少值")
			}
			options.realm = args[index]
		default:
			if strings.HasPrefix(argument, "-") {
				return moduleOptions{}, fmt.Errorf("未知参数 %q", argument)
			}
			if options.domain != "" {
				return moduleOptions{}, fmt.Errorf("module new 只接受一个领域名称")
			}
			options.domain = argument
		}
	}
	if !moduleNamePattern.MatchString(options.domain) {
		return moduleOptions{}, fmt.Errorf("领域名称必须匹配 %s", moduleNamePattern.String())
	}
	if options.realm != "none" && options.realm != "app" && options.realm != "admin" {
		return moduleOptions{}, fmt.Errorf("realm 只允许 app admin 或 none")
	}
	return options, nil
}

func renderModuleFiles(directory string, projectModule string, options moduleOptions) ([]generatedFile, error) {
	apiImport := "\t\"github.com/gofiber/fiber/v3\"\n"
	realmParameter := ""
	if options.realm != "none" {
		apiImport = fmt.Sprintf("\t%q\n\n%s", projectModule+"/internal/auth", apiImport)
		realmParameter = ", realm *auth.Realm"
	}
	apiSource := fmt.Sprintf(`package %s

import (
%s)

// RegisterHandlers 注册 %s 模块路由
func RegisterHandlers(router fiber.Router, service *Service%s) {
	// 路由由具体业务动作补充
}
`, options.domain, apiImport, options.domain, realmParameter)
	serviceSource := fmt.Sprintf(`package %s

// Service 提供 %s 模块业务能力
type Service struct {
	repository *Repository
}

// NewService 创建 %s 模块服务
func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}
`, options.domain, options.domain, options.domain)
	repositorySource := fmt.Sprintf(`package %s

import %q

// Repository 使用 GORM 实现 %s 模块持久化
type Repository struct {
	data *data.Data
}

// NewRepository 创建 %s 模块 Repository
func NewRepository(resources *data.Data) *Repository {
	return &Repository{data: resources}
}
`, options.domain, projectModule+"/internal/data", options.domain, options.domain)

	sources := []struct {
		name   string
		source string
	}{
		{name: "api.go", source: apiSource},
		{name: "service.go", source: serviceSource},
		{name: "repository.go", source: repositorySource},
	}
	files := make([]generatedFile, 0, len(sources))
	for _, source := range sources {
		formatted, err := format.Source([]byte(source.source))
		if err != nil {
			return nil, fmt.Errorf("格式化 %s: %w", source.name, err)
		}
		files = append(files, generatedFile{
			path:    filepath.Join(directory, source.name),
			content: formatted,
		})
	}
	return files, nil
}

func updateModulesSource(source []byte, projectModule string, options moduleOptions) ([]byte, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "modules.go", source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("解析 internal/app/modules.go: %w", err)
	}
	importPath := projectModule + "/internal/modules/" + options.domain
	for _, imported := range file.Imports {
		if imported.Path.Value == strconv.Quote(importPath) {
			return nil, fmt.Errorf("模块已在 internal/app/modules.go 中导入: %s", options.domain)
		}
	}

	var importDeclaration *ast.GenDecl
	for _, declaration := range file.Decls {
		candidate, ok := declaration.(*ast.GenDecl)
		if ok && candidate.Tok == token.IMPORT && candidate.Lparen.IsValid() {
			importDeclaration = candidate
			break
		}
	}
	if importDeclaration == nil {
		return nil, fmt.Errorf("internal/app/modules.go 缺少 import 块")
	}

	var registerFunction *ast.FuncDecl
	for _, declaration := range file.Decls {
		candidate, ok := declaration.(*ast.FuncDecl)
		if ok && candidate.Name.Name == "registerModules" && candidate.Body != nil {
			registerFunction = candidate
			break
		}
	}
	if registerFunction == nil {
		return nil, fmt.Errorf("internal/app/modules.go 缺少 registerModules")
	}

	bodyOffset := fileSet.Position(registerFunction.Body.Rbrace).Offset
	for _, statement := range registerFunction.Body.List {
		if calledPackage(statement) == "healthmodule" {
			bodyOffset = precedingCommentStart(source, lineStart(source, fileSet.Position(statement.Pos()).Offset))
			break
		}
	}
	alias := options.domain + "module"
	assemblyCall := fmt.Sprintf("%s.RegisterHandlers(router, %sService)", alias, options.domain)
	if options.realm == "app" {
		assemblyCall = fmt.Sprintf("%s.RegisterHandlers(router, %sService, realms.App)", alias, options.domain)
	} else if options.realm == "admin" {
		assemblyCall = fmt.Sprintf("%s.RegisterHandlers(router, %sService, realms.Admin)", alias, options.domain)
	}
	assembly := fmt.Sprintf(
		"\t// 装配 %s 模块\n\t%sService := %s.NewService(%s.NewRepository(resources))\n\t%s\n\n",
		options.domain,
		options.domain,
		alias,
		alias,
		assemblyCall,
	)
	importLine := fmt.Sprintf("\t%s %q\n", alias, importPath)
	importOffset := importInsertionOffset(fileSet, source, file.Imports, projectModule, importPath)
	if importOffset == 0 {
		importOffset = fileSet.Position(importDeclaration.Rparen).Offset
	}

	updated := insertAt(source, bodyOffset, []byte(assembly))
	updated = insertAt(updated, importOffset, []byte(importLine))
	formatted, err := format.Source(updated)
	if err != nil {
		return nil, fmt.Errorf("格式化 internal/app/modules.go: %w", err)
	}
	return formatted, nil
}

func calledPackage(statement ast.Stmt) string {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return ""
	}
	call, ok := expression.X.(*ast.CallExpr)
	if !ok {
		return ""
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return identifier.Name
}

func lineStart(source []byte, offset int) int {
	for offset > 0 && source[offset-1] != '\n' {
		offset--
	}
	return offset
}

func precedingCommentStart(source []byte, offset int) int {
	for offset > 0 {
		previousEnd := offset - 1
		previousStart := lineStart(source, previousEnd)
		line := strings.TrimSpace(string(source[previousStart:previousEnd]))
		if !strings.HasPrefix(line, "//") {
			break
		}
		offset = previousStart
	}
	return offset
}

func importInsertionOffset(
	fileSet *token.FileSet,
	source []byte,
	imports []*ast.ImportSpec,
	projectModule string,
	newPath string,
) int {
	lastInternalEnd := 0
	for _, imported := range imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil || !strings.HasPrefix(path, projectModule+"/") {
			continue
		}
		start := lineStart(source, fileSet.Position(imported.Pos()).Offset)
		if path > newPath {
			return start
		}
		end := fileSet.Position(imported.End()).Offset
		for end < len(source) && source[end] != '\n' {
			end++
		}
		if end < len(source) {
			end++
		}
		lastInternalEnd = end
	}
	return lastInternalEnd
}

func insertAt(source []byte, offset int, insertion []byte) []byte {
	result := make([]byte, 0, len(source)+len(insertion))
	result = append(result, source[:offset]...)
	result = append(result, insertion...)
	result = append(result, source[offset:]...)
	return result
}

func commitModule(
	directory string,
	files []generatedFile,
	modulesPath string,
	modulesSource []byte,
) error {
	if err := os.MkdirAll(filepath.Dir(directory), 0o755); err != nil {
		return fmt.Errorf("创建 modules 目录: %w", err)
	}
	if err := os.Mkdir(directory, 0o755); err != nil {
		return fmt.Errorf("创建模块目录: %w", err)
	}
	created := make([]string, 0, len(files))
	rollback := func() {
		for _, path := range created {
			_ = os.Remove(path)
		}
		_ = os.Remove(directory)
	}
	for _, file := range files {
		if err := writeExclusive(file.path, file.content); err != nil {
			rollback()
			return err
		}
		created = append(created, file.path)
	}
	if err := replaceFile(modulesPath, modulesSource); err != nil {
		rollback()
		return err
	}
	return nil
}

func replaceFile(path string, content []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("读取 %s 元数据: %w", path, err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".modules-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(info.Mode()); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("设置临时文件权限: %w", err)
	}
	if _, err := bytes.NewReader(content).WriteTo(temporary); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("写入临时文件: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭临时文件: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("更新 %s: %w", path, err)
	}
	return nil
}
