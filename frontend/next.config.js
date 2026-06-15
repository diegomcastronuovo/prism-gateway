/** @type {import('next').NextConfig} */
const nextConfig = {
  // Standalone output reduces image size by ~90%
  // Creates a minimal runtime package without node_modules
  // Enable for production containerization
  output: 'standalone',

  // Disable ESLint during build to allow Docker builds to succeed
  // ESLint errors are still shown during development (npm run dev)
  eslint: {
    ignoreDuringBuilds: true,
  },
}

module.exports = nextConfig
