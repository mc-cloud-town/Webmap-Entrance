FROM node:20 as builder

WORKDIR /app

COPY package.json .
COPY yarn.lock .

RUN yarn install

COPY . .
RUN yarn build

FROM node:20 AS production

WORKDIR /app

COPY --from=builder /app .

CMD yarn start

EXPOSE 3000
