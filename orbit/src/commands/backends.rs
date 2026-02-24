use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};

const BACKEND_COLUMNS: &[Column] = &[
    Column::new("Name", "name"),
    Column::new("Type", "type"),
    Column::new("Status", "status"),
];

pub async fn run(client: &NovaClient, output_format: &str) -> Result<()> {
    let result = client.get("/backends").await?;
    output::render(&result, BACKEND_COLUMNS, output_format);
    Ok(())
}
