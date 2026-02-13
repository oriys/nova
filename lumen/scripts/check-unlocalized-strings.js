#!/usr/bin/env node

/* eslint-disable no-console */

const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")

const cwd = process.cwd()
const targetDirs = process.argv.slice(2).length > 0 ? process.argv.slice(2) : ["app", "components", "lib"]

const SOURCE_EXTENSIONS = new Set([".ts", ".tsx", ".js", ".jsx"])
const IGNORE_DIRS = new Set(["node_modules", ".next", "messages", "public", "dist", "build", "coverage", "out"])
const JSX_VISIBLE_ATTRS = new Set([
  "title",
  "placeholder",
  "alt",
  "aria-label",
  "aria-description",
  "label",
  "helperText",
  "description",
])
const USER_MESSAGE_CALLS = new Set([
  "toast",
  "toast.success",
  "toast.error",
  "toast.info",
  "toast.warning",
  "alert",
  "confirm",
  "prompt",
  "message.success",
  "message.error",
  "message.info",
  "notification.success",
  "notification.error",
  "notification.info",
])

const DIRECTIVE_FILE = "i18n-ignore-file"
const DIRECTIVE_LINE = "i18n-ignore-line"
const DIRECTIVE_NEXT = "i18n-ignore-next-line"

function getScriptKind(filePath) {
  if (filePath.endsWith(".tsx")) return ts.ScriptKind.TSX
  if (filePath.endsWith(".jsx")) return ts.ScriptKind.JSX
  if (filePath.endsWith(".ts")) return ts.ScriptKind.TS
  return ts.ScriptKind.JS
}

function getCalleeName(expression) {
  if (ts.isIdentifier(expression)) return expression.text
  if (ts.isPropertyAccessExpression(expression)) {
    const left = getCalleeName(expression.expression)
    return left ? `${left}.${expression.name.text}` : expression.name.text
  }
  return ""
}

function isKeyLike(text) {
  return /^[a-z0-9_.-]+$/i.test(text) && (text.includes(".") || text.includes("_"))
}

function isProbablyHumanText(value) {
  const text = value.replace(/\s+/g, " ").trim()
  if (!text) return false

  if (/^(true|false|null|undefined)$/i.test(text)) return false
  if (/^(https?:\/\/|mailto:|tel:)/i.test(text)) return false
  if (/^[#./@$_:+\-0-9]+$/.test(text)) return false
  if (/^[A-Z0-9_]+$/.test(text)) return false
  if (/^--[a-z0-9-]+$/i.test(text)) return false
  if (isKeyLike(text)) return false
  if (/^[a-z0-9-]+\/[a-z0-9-_.]+$/i.test(text)) return false

  const hasLanguageChar = /[\p{L}]/u.test(text)
  if (!hasLanguageChar) return false

  const hasCjk = /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}]/u.test(text)
  if (text.length <= 2 && !hasCjk) return false

  return true
}

function normalizeSnippet(text) {
  return text.replace(/\s+/g, " ").trim().slice(0, 100)
}

function collectFiles(rootDirs) {
  const files = []
  for (const dir of rootDirs) {
    const absolute = path.resolve(cwd, dir)
    if (!fs.existsSync(absolute)) continue
    walk(absolute, files)
  }
  return files
}

function walk(currentPath, files) {
  const stat = fs.statSync(currentPath)
  if (stat.isDirectory()) {
    if (IGNORE_DIRS.has(path.basename(currentPath))) return
    const entries = fs.readdirSync(currentPath)
    for (const entry of entries) {
      walk(path.join(currentPath, entry), files)
    }
    return
  }

  const ext = path.extname(currentPath).toLowerCase()
  if (!SOURCE_EXTENSIONS.has(ext)) return
  if (currentPath.endsWith(".d.ts")) return
  files.push(currentPath)
}

function readDirectives(sourceText) {
  const lines = sourceText.split(/\r?\n/)

  return {
    fileIgnored: lines.some((line) => line.includes(DIRECTIVE_FILE)),
    shouldIgnoreLine(lineNo) {
      const line = lines[lineNo - 1] || ""
      const prev = lines[lineNo - 2] || ""
      return line.includes(DIRECTIVE_LINE) || prev.includes(DIRECTIVE_NEXT)
    },
  }
}

function scanFile(filePath) {
  const sourceText = fs.readFileSync(filePath, "utf8")
  const directives = readDirectives(sourceText)
  if (directives.fileIgnored) return []

  const source = ts.createSourceFile(filePath, sourceText, ts.ScriptTarget.Latest, true, getScriptKind(filePath))
  const findings = []

  function report(node, type, value) {
    const pos = source.getLineAndCharacterOfPosition(node.getStart(source))
    const lineNo = pos.line + 1
    if (directives.shouldIgnoreLine(lineNo)) return

    findings.push({
      file: filePath,
      line: lineNo,
      col: pos.character + 1,
      type,
      text: normalizeSnippet(value),
    })
  }

  function isStringLiteralNode(node) {
    return ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)
  }

  function visit(node) {
    if (ts.isJsxText(node)) {
      const text = node.getText(source).replace(/\s+/g, " ").trim()
      if (isProbablyHumanText(text)) {
        report(node, "jsx-text", text)
      }
    }

    if (ts.isJsxAttribute(node)) {
      const attrName = node.name.getText(source)
      if (JSX_VISIBLE_ATTRS.has(attrName) && node.initializer) {
        if (isStringLiteralNode(node.initializer) && isProbablyHumanText(node.initializer.text)) {
          report(node.initializer, `jsx-attr:${attrName}`, node.initializer.text)
        }
        if (ts.isJsxExpression(node.initializer) && node.initializer.expression) {
          const expr = node.initializer.expression
          if (isStringLiteralNode(expr) && isProbablyHumanText(expr.text)) {
            report(expr, `jsx-attr:${attrName}`, expr.text)
          }
        }
      }
    }

    if (isStringLiteralNode(node)) {
      const parent = node.parent
      const insideJsxExpression =
        ts.isJsxExpression(parent) &&
        (ts.isJsxElement(parent.parent) || ts.isJsxFragment(parent.parent) || ts.isJsxSelfClosingElement(parent.parent))

      if (
        !ts.isImportDeclaration(parent) &&
        !ts.isExportDeclaration(parent) &&
        !ts.isImportEqualsDeclaration(parent) &&
        !ts.isExternalModuleReference(parent) &&
        insideJsxExpression &&
        isProbablyHumanText(node.text)
      ) {
        report(node, "jsx-expression-text", node.text)
      }
    }

    if (ts.isCallExpression(node)) {
      const callee = getCalleeName(node.expression)
      if (USER_MESSAGE_CALLS.has(callee) && node.arguments.length > 0) {
        const firstArg = node.arguments[0]
        if (isStringLiteralNode(firstArg) && isProbablyHumanText(firstArg.text)) {
          report(firstArg, `call:${callee}`, firstArg.text)
        }
      }
    }

    ts.forEachChild(node, visit)
  }

  visit(source)
  return findings
}

function main() {
  const files = collectFiles(targetDirs)
  let findings = []
  for (const file of files) {
    findings = findings.concat(scanFile(file))
  }

  findings.sort((a, b) => a.file.localeCompare(b.file) || a.line - b.line || a.col - b.col)

  if (findings.length === 0) {
    console.log("No obvious hardcoded user-facing strings found.")
    return
  }

  console.log(`Found ${findings.length} potential non-i18n strings:\n`)
  for (const finding of findings) {
    console.log(
      `${path.relative(cwd, finding.file)}:${finding.line}:${finding.col}  [${finding.type}]  "${finding.text}"`,
    )
  }

  console.log("\nTips:")
  console.log('- Replace user-facing text with t("...") keys from messages/*.json')
  console.log(`- Add // ${DIRECTIVE_NEXT} to suppress intentional literals`)
  process.exitCode = 1
}

main()
