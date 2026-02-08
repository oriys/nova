use serde::{Deserialize, Serialize};
use std::path::PathBuf;

#[derive(Debug, Serialize, Deserialize, Default)]
pub struct OrbitConfig {
    pub server: Option<String>,
    pub api_key: Option<String>,
    pub tenant: Option<String>,
    pub namespace: Option<String>,
    pub output: Option<String>,
}

impl OrbitConfig {
    pub fn load() -> Self {
        let path = Self::config_path();
        if path.exists() {
            let content = std::fs::read_to_string(&path).unwrap_or_default();
            toml::from_str(&content).unwrap_or_default()
        } else {
            Self::default()
        }
    }

    pub fn save(&self) -> crate::error::Result<()> {
        let path = Self::config_path();
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)?;
        }
        let content = toml::to_string_pretty(self)
            .map_err(|e| crate::error::OrbitError::Config(e.to_string()))?;
        std::fs::write(&path, content)?;
        Ok(())
    }

    fn config_path() -> PathBuf {
        dirs::home_dir()
            .unwrap_or_else(|| PathBuf::from("."))
            .join(".orbit")
            .join("config.toml")
    }
}
