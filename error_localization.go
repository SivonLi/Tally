package main

import (
	"strings"

	"github.com/egoist/mygo/i18n"
)

var internalErrorMap = map[string]string{
	"WebDAV 服务器地址不能为空":              "WebDAV server URL is required",
	"URL 必须使用 https 或 http 协议":      "URL must use https or http protocol",
	"仅允许 HTTPS 或局域网内网 HTTP 连接":      "Only HTTPS or local network HTTP connections are allowed",
	"WebDAV 用户名或密码错误 (401)":         "WebDAV username or password error (401)",
	"WebDAV 目录权限不足 (403)":           "WebDAV directory access denied (403)",
	"WebDAV 目录或文件不存在 (404)":         "WebDAV directory or file not found (404)",
	"WebDAV 服务器空间不足 (507)":          "WebDAV server insufficient storage (507)",
	"不支持的数据包格式版本":                   "Unsupported package format version",
	"不支持的加密版本或算法":                   "Unsupported encryption version or algorithm",
	"加密密码不能为空":                      "Encryption password cannot be empty",
	"开启加密时同步密码不能为空":                 "Sync password cannot be empty when encryption is enabled",
	"远端数据包已加密，请输入同步密码":              "Remote package is encrypted, please enter sync password",
	"数据包解密失败，请检查同步密码是否正确":           "Failed to decrypt package, please check if sync password is correct",
	"远端数据包哈希与 latest.json 记录不匹配":    "Remote package hash does not match latest.json record",
	"远端数据包校验哈希不匹配，已自动回滚删除":          "Remote package verification hash mismatch, rolled back and deleted",
	"明文哈希校验不匹配":                     "Plaintext hash verification mismatch",
	"数据包缺少数据库文件":                    "Package missing database file",
	"下载的数据未通过 SQLite 完整性检查，放弃替换":    "Downloaded data failed SQLite integrity check, replacement aborted",
	"本次考核还没有核算结果":                   "This assessment does not have calculation results yet",
	"此导入之后还有新的导入，不能撤销":              "There are newer imports after this import, cannot undo",
	"只有基础数据导入可以撤销":                  "Only foundational data imports can be undone",
	"导入存在阻断问题":                      "Import contains blocking issues",
	"该文件已经导入过这类数据":                  "This file has already been imported for this data kind",
	"级已经存在":                         "Cohort already exists",
	"考试已经存在":                        "Exam already exists",
	"考试没有这个级":                       "Exam does not include this cohort",
	"记录不存在":                         "Record does not exist",
	"不参与的原因不能为空":                    "Reason for non-participation cannot be empty",
	"个人得分的权重为零":                     "Personal score weight is zero",
	"参评统计组数量必须大于零":                  "Number of participating stat groups must be greater than zero",
	"报名名单缺少选科类别":                    "Roster is missing track selection",
	"入学年份超出允许范围":                    "Enrollment year is out of allowed range",
	"学年写法无法解析":                      "Academic year format cannot be parsed",
	"年级阶段无法识别":                      "Grade stage cannot be recognized",
	"没有找到 LibreOffice 的 soffice":    "LibreOffice soffice executable not found",
	"salt 不能为空":                     "Salt cannot be empty",
	"缺少数据库文件的 nonce 记录":             "Missing database file nonce record",
	"无效的 nonce 记录":                  "Invalid nonce record",
	"无效的加密容器文件头":                    "Invalid encrypted container header",
	"无效的加密盐值":                       "Invalid encryption salt",
	"system plan cannot be retired": "System plan cannot be retired",
	"缺少必须的列":                        "Missing required columns",
}

var internalErrorPrefixMap = []struct {
	prefix      string
	replacement string
}{
	{"创建上传快照失败: ", "Failed to create upload snapshot: "},
	{"读取快照数据失败: ", "Failed to read snapshot data: "},
	{"打开校验临时数据库失败: ", "Failed to open temporary database for verification: "},
	{"备份当前数据库失败: ", "Failed to backup current database: "},
	{"上传临时指针文件失败: ", "Failed to upload temporary pointer file: "},
	{"上传原始文件 ", "Failed to upload original file "},
	{"上传数据包失败: ", "Failed to upload package: "},
	{"上传文件失败，状态码: ", "Failed to upload file, status code: "},
	{"下载文件失败，状态码: ", "Failed to download file, status code: "},
	{"下载远端数据包失败: ", "Failed to download remote package: "},
	{"创建数据包失败: ", "Failed to create package: "},
	{"创建请求失败: ", "Failed to create request: "},
	{"删除文件失败，状态码: ", "Failed to delete file, status code: "},
	{"加密数据库快照失败: ", "Failed to encrypt database snapshot: "},
	{"加密原始文件 ", "Failed to encrypt original file "},
	{"保存导出文件: ", "Save export file: "},
	{"创建导出目录: ", "Create export directory: "},
	{"创建转换目录: ", "Create conversion directory: "},
	{"创建独立配置目录: ", "Create independent config directory: "},
	{"入口成绩数据集写入失败", "Failed to write baseline dataset"},
}

// localizeErrorMessage maps internal error text to the current locale.
// When the locale is English (or non-Chinese), it translates known internal error strings.
func localizeErrorMessage(msg string) string {
	if msg == "" {
		return ""
	}
	loc := i18n.Locale()
	if strings.HasPrefix(loc, "zh") {
		return msg
	}
	// English / other locales
	if translated, ok := internalErrorMap[msg]; ok {
		return translated
	}
	for _, rule := range internalErrorPrefixMap {
		if strings.HasPrefix(msg, rule.prefix) {
			rest := strings.TrimPrefix(msg, rule.prefix)
			return rule.replacement + rest
		}
	}
	return msg
}

func (a *app) localizeError(err error) string {
	if err == nil {
		return ""
	}
	return localizeErrorMessage(err.Error())
}
