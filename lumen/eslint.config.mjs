import tseslint from "typescript-eslint";
import js from "@eslint/js";
import i18next from "eslint-plugin-i18next";

export default tseslint.config(
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    ignores: [".next/", "node_modules/", "components/ui/"],
  },
  {
    rules: {
      "@typescript-eslint/no-unused-vars": ["warn", { argsIgnorePattern: "^_" }],
      "@typescript-eslint/no-explicit-any": "off",
      "@typescript-eslint/no-empty-object-type": "off",
      "prefer-const": "warn",
      "no-console": ["warn", { allow: ["error", "warn"] }],
    },
  },
  {
    files: ["app/**/*.{ts,tsx}", "components/**/*.{ts,tsx}"],
    ignores: ["components/ui/**"],
    plugins: { i18next },
    rules: {
      "i18next/no-literal-string": "warn",
    },
  },
);
