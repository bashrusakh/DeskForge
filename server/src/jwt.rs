use jsonwebtoken::{decode, encode, Algorithm, DecodingKey, EncodingKey, Header, Validation};
use once_cell::sync::Lazy;
use serde::{Deserialize, Serialize};
use std::env;
pub static SECRET: Lazy<String> =
    Lazy::new(|| env::var("RUSTDESK_API_JWT_KEY").unwrap_or_else(|_| "".to_string()));

// Struct representing the JWT payload
#[derive(Debug, Serialize, Deserialize)]
pub struct Claims {
    user_id: u32,
    exp: usize,
}

pub fn generate_token(user_id: u32, exp: i64) -> Result<String, String> {
    println!("secret: {:}", SECRET.to_string());
    let claims = Claims {
        user_id,
        exp: (chrono::Utc::now() + chrono::Duration::seconds(exp)).timestamp() as usize,
    };

    let token = encode(
        &Header::default(),
        &claims,
        &EncodingKey::from_secret(SECRET.as_ref()),
    );

    match token {
        Ok(t) => Ok(t),
        Err(e) => Err(e.to_string()),
    }
}
// Verify JWT token
pub fn verify_token(token: &str) -> Result<Claims, String> {
    // Decode JWT
    let validation = Validation::new(Algorithm::HS256);

    let decoded = decode::<Claims>(
        &token,
        &DecodingKey::from_secret(SECRET.as_ref()),
        &validation,
    );
    match decoded {
        Ok(token_data) => {
            let now = chrono::Utc::now().timestamp() as usize;
            if token_data.claims.exp > now {
                Ok(token_data.claims)
            } else {
                Err("Token status invalid or expired".to_string())
            }
        }
        Err(_) => Err("Invalid token".to_string()),
    }
}
