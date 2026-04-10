FROM node:22-alpine AS builder
WORKDIR /app

COPY apps/meza-panel/package.json ./
COPY apps/meza-panel/package-lock.json ./
RUN npm ci

COPY apps/meza-panel/app ./app
COPY apps/meza-panel/components ./components
COPY apps/meza-panel/lib ./lib
COPY apps/meza-panel/next-env.d.ts ./
COPY apps/meza-panel/next.config.ts ./
COPY apps/meza-panel/tsconfig.json ./

RUN npm run build

FROM node:22-alpine
WORKDIR /app

ENV NODE_ENV=production

COPY --from=builder /app/package.json ./package.json
COPY --from=builder /app/package-lock.json ./package-lock.json
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/.next ./.next
COPY --from=builder /app/app ./app
COPY --from=builder /app/components ./components
COPY --from=builder /app/lib ./lib
COPY --from=builder /app/next.config.ts ./next.config.ts

EXPOSE 3000

CMD ["npm", "run", "start", "--", "--hostname", "0.0.0.0", "--port", "3000"]
