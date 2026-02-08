use crate::error::{OrbitError, Result};
use reqwest::{Client, Method, Response};
use serde_json::Value;

pub struct NovaClient {
    client: Client,
    base_url: String,
    api_key: Option<String>,
    tenant: Option<String>,
    namespace: Option<String>,
}

impl NovaClient {
    pub fn new(
        base_url: String,
        api_key: Option<String>,
        tenant: Option<String>,
        namespace: Option<String>,
    ) -> Self {
        Self {
            client: Client::new(),
            base_url: base_url.trim_end_matches('/').to_string(),
            api_key,
            tenant,
            namespace,
        }
    }

    fn build_request(&self, method: Method, path: &str) -> reqwest::RequestBuilder {
        let url = format!("{}{}", self.base_url, path);
        let mut req = self.client.request(method, &url);
        if let Some(key) = &self.api_key {
            req = req.header("X-API-Key", key);
        }
        if let Some(t) = &self.tenant {
            req = req.header("X-Tenant-ID", t);
        }
        if let Some(ns) = &self.namespace {
            req = req.header("X-Namespace", ns);
        }
        req
    }

    async fn handle_response(resp: Response) -> Result<Value> {
        let status = resp.status().as_u16();
        if status >= 400 {
            let body = resp.text().await.unwrap_or_default();
            let message = serde_json::from_str::<Value>(&body)
                .ok()
                .and_then(|v| v.get("error").and_then(|e| e.as_str()).map(String::from))
                .unwrap_or(body);
            return Err(OrbitError::api(status, message));
        }
        let text = resp.text().await?;
        if text.is_empty() {
            Ok(Value::Null)
        } else {
            serde_json::from_str(&text).map_err(OrbitError::Json)
        }
    }

    pub async fn get(&self, path: &str) -> Result<Value> {
        let resp = self.build_request(Method::GET, path).send().await?;
        Self::handle_response(resp).await
    }

    pub async fn post(&self, path: &str, body: &Value) -> Result<Value> {
        let resp = self
            .build_request(Method::POST, path)
            .json(body)
            .send()
            .await?;
        Self::handle_response(resp).await
    }

    pub async fn patch(&self, path: &str, body: &Value) -> Result<Value> {
        let resp = self
            .build_request(Method::PATCH, path)
            .json(body)
            .send()
            .await?;
        Self::handle_response(resp).await
    }

    pub async fn put(&self, path: &str, body: &Value) -> Result<Value> {
        let resp = self
            .build_request(Method::PUT, path)
            .json(body)
            .send()
            .await?;
        Self::handle_response(resp).await
    }

    pub async fn delete(&self, path: &str) -> Result<Value> {
        let resp = self.build_request(Method::DELETE, path).send().await?;
        Self::handle_response(resp).await
    }

}
