export const authConfig = {
  keycloak: {
    url: process.env.NEXT_PUBLIC_KEYCLOAK_URL || '',
    realm: process.env.NEXT_PUBLIC_KEYCLOAK_REALM || '',
    clientId: process.env.NEXT_PUBLIC_KEYCLOAK_CLIENT_ID || '',
  },
  cognito: {
    region: process.env.NEXT_PUBLIC_COGNITO_REGION || '',
    userPoolId: process.env.NEXT_PUBLIC_COGNITO_USER_POOL_ID || '',
    clientId: process.env.NEXT_PUBLIC_COGNITO_CLIENT_ID || '',
  },
  redirectUri: process.env.NEXT_PUBLIC_AUTH_REDIRECT_URI || 'http://localhost:3000',
}
