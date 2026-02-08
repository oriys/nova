use comfy_table::{modifiers::UTF8_ROUND_CORNERS, presets::UTF8_FULL, Table, ContentArrangement};
use serde_json::Value;

pub struct Column {
    pub header: &'static str,
    pub path: &'static str,
    pub wide_only: bool,
}

impl Column {
    pub const fn new(header: &'static str, path: &'static str) -> Self {
        Self {
            header,
            path,
            wide_only: false,
        }
    }

    pub const fn wide(header: &'static str, path: &'static str) -> Self {
        Self {
            header,
            path,
            wide_only: true,
        }
    }
}

fn extract_field(value: &Value, path: &str) -> String {
    let mut current = value;
    for key in path.split('.') {
        match current {
            Value::Object(map) => {
                current = map.get(key).unwrap_or(&Value::Null);
            }
            Value::Array(arr) => {
                if let Ok(idx) = key.parse::<usize>() {
                    current = arr.get(idx).unwrap_or(&Value::Null);
                } else {
                    return "-".to_string();
                }
            }
            _ => return "-".to_string(),
        }
    }
    match current {
        Value::Null => "-".to_string(),
        Value::String(s) => s.clone(),
        Value::Bool(b) => b.to_string(),
        Value::Number(n) => n.to_string(),
        Value::Array(arr) => {
            if arr.is_empty() {
                "-".to_string()
            } else {
                let items: Vec<String> = arr
                    .iter()
                    .map(|v| match v {
                        Value::String(s) => s.clone(),
                        other => other.to_string(),
                    })
                    .collect();
                items.join(", ")
            }
        }
        Value::Object(_) => serde_json::to_string(current).unwrap_or_else(|_| "-".to_string()),
    }
}

pub fn render(data: &Value, columns: &[Column], format: &str) {
    match format {
        "json" => {
            println!(
                "{}",
                serde_json::to_string_pretty(data).unwrap_or_else(|_| data.to_string())
            );
        }
        "yaml" => {
            println!(
                "{}",
                serde_yaml::to_string(data).unwrap_or_else(|_| data.to_string())
            );
        }
        _ => {
            let wide = format == "wide";
            let active_columns: Vec<&Column> = columns
                .iter()
                .filter(|c| wide || !c.wide_only)
                .collect();

            match data {
                Value::Array(items) => {
                    if items.is_empty() {
                        println!("No resources found.");
                        return;
                    }
                    let mut table = Table::new();
                    table
                        .load_preset(UTF8_FULL)
                        .apply_modifier(UTF8_ROUND_CORNERS)
                        .set_content_arrangement(ContentArrangement::Dynamic);

                    let headers: Vec<&str> =
                        active_columns.iter().map(|c| c.header).collect();
                    table.set_header(headers);

                    for item in items {
                        let row: Vec<String> = active_columns
                            .iter()
                            .map(|c| extract_field(item, c.path))
                            .collect();
                        table.add_row(row);
                    }
                    println!("{table}");
                }
                Value::Object(_) => {
                    let mut table = Table::new();
                    table
                        .load_preset(UTF8_FULL)
                        .apply_modifier(UTF8_ROUND_CORNERS)
                        .set_content_arrangement(ContentArrangement::Dynamic);
                    table.set_header(vec!["Field", "Value"]);
                    for col in &active_columns {
                        table.add_row(vec![col.header.to_string(), extract_field(data, col.path)]);
                    }
                    println!("{table}");
                }
                _ => {
                    println!("{}", serde_json::to_string_pretty(data).unwrap_or_else(|_| data.to_string()));
                }
            }
        }
    }
}

pub fn render_single(data: &Value, columns: &[Column], format: &str) {
    render(data, columns, format);
}

pub fn print_success(msg: &str) {
    use colored::Colorize;
    println!("{}", msg.green());
}

pub fn print_error(msg: &str) {
    use colored::Colorize;
    eprintln!("{}", msg.red());
}
