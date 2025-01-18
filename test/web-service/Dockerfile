FROM node:18-alpine

WORKDIR /app

# Force IPv4 for npm
ENV NODE_OPTIONS="--dns-result-order=ipv4first"

# Debug: List contents before copy
RUN pwd && ls -la

# Copy package files individually
COPY package.json .
COPY package-lock.json .

# Debug: List contents after copy
RUN ls -la

# Install dependencies
RUN npm install --verbose

# Copy rest of the files
COPY . .

EXPOSE 3000
CMD ["npm", "start"]
