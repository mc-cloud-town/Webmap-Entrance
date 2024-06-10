FROM node:20 as builder

WORKDIR /app

COPY package.json .
COPY yarn.lock .

RUN yarn install

COPY . .
RUN yarn build

FROM node:20 AS production

COPY --from=builder /app/package.json .
COPY --from=builder /app/node_modules .
COPY --from=builder /app/dist ./dist
COPY ./public .

CMD yarn start

EXPOSE 3000
