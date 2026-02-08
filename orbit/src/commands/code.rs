use crate::client::NovaClient;
use crate::commands::functions::CodeSubCmd;
use crate::error::Result;
use crate::output;
use serde_json::json;

pub async fn run(cmd: CodeSubCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        CodeSubCmd::Get { name } => {
            let result = client.get(&format!("/functions/{name}/code")).await?;
            if output_format == "table" || output_format == "wide" {
                if let Some(code) = result.get("code").and_then(|v| v.as_str()) {
                    println!("{code}");
                } else {
                    println!("{}", serde_json::to_string_pretty(&result)?);
                }
            } else {
                output::render_single(&result, &[], output_format);
            }
        }
        CodeSubCmd::Update { name, code, file } => {
            let code_value = match (code, file) {
                (Some(c), _) => c,
                (_, Some(path)) => std::fs::read_to_string(&path).map_err(|e| {
                    crate::error::OrbitError::Input(format!("Cannot read file {path}: {e}"))
                })?,
                _ => {
                    return Err(crate::error::OrbitError::Input(
                        "Provide --code or --file".into(),
                    ));
                }
            };
            let body = json!({ "code": code_value });
            let result = client
                .put(&format!("/functions/{name}/code"), &body)
                .await?;
            output::print_success(&format!("Code updated for '{name}'."));
            if output_format == "json" || output_format == "yaml" {
                output::render_single(&result, &[], output_format);
            }
        }
    }
    Ok(())
}
