# AI Prompt Templates

Nova AI prompts are managed centrally in this directory.

- `platform_context.tmpl`: Shared Nova platform context used by code generation/review/rewrite.
- `generate_system.tmpl`, `generate_user.tmpl`: Code generation prompts.
- `review_system_*.tmpl`, `review_user.tmpl`: Code review prompts.
- `rewrite_system.tmpl`, `rewrite_user.tmpl`: Code rewrite prompts.
- `diagnostics_system.tmpl`, `diagnostics_user.tmpl`: Diagnostics analysis prompts.

Runtime behavior:
- Default prompt directory: `configs/prompts/ai`
- Override with env var: `NOVA_AI_PROMPT_DIR`
- API config field: `prompt_dir`
- If a file is missing in `prompt_dir`, the built-in embedded template is used as fallback.
