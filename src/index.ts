import path from 'path';

import 'dotenv/config';

import session from 'express-session';
import express from 'express';
import { createProxyMiddleware } from 'http-proxy-middleware';
import { request } from 'undici';
import { Client, Events, GatewayIntentBits } from 'discord.js';
import type { Request, Response } from 'express';
import cors from 'cors';

export interface User {
  id: string;
  username: string;
  avatar: string;
}

export interface DiscordOauth2Token {
  token_type: 'Bearer' | 'Bot';
  access_token: string;
  expires_in: number;
  refresh_token: string;
  scope: string;
}

declare module 'express-session' {
  interface SessionData {
    user?: User;
  }
}

const CTEC_DISCORD_GUILD_ID = '933290709589577728';
const CTEC_DISCORD_INTERNAL_ROLES = [
  '933382711148695673', // 雲鎮伙伴-member
  '1049504039211118652', // 二審中-trialing
];
const port = process.env.PORT || 3000;
const DISCORD_TOKEN = process.env.DISCORD_TOKEN ?? '';
const DISCORD_CLIENT_ID = process.env.DISCORD_CLIENT_ID ?? '';
const DISCORD_CLIENT_SECRET = process.env.DISCORD_CLIENT_SECRET ?? '';
const DISCORD_CLIENT_REDIRECT_URI =
  process.env.DISCORD_CLIENT_REDIRECT_URI ??
  `http://localhost:${port}/callback`;
// const WEB_MAP_URL = process.env.WEB_MAP_URL ?? '';

const client = new Client({
  intents: [GatewayIntentBits.Guilds, GatewayIntentBits.GuildMembers],
});

const app = express();

const WEB_MAP_PROXY = createProxyMiddleware<Request, Response>({
  target: process.env.WEB_MAP_URL || 'http://localhost:3000',
  changeOrigin: true,
});
// https://discord.com/oauth2/authorize?client_id=1200342075238006874&response_type=code&redirect_uri=http%3A%2F%2Flocalhost%3A3000%2Fcallback&scope=identify

const hasCTECMember = async (userID?: string) => {
  if (!userID) {
    return false;
  }

  const members = client.guilds.cache.get(CTEC_DISCORD_GUILD_ID)?.members;
  if (!members) {
    return false;
  }

  let user_ = members.cache.get(userID);
  if (!user_) {
    try {
      user_ = await members.fetch(userID);
    } catch (error) {
      return false;
    }
  }

  if (user_) {
    const roles = user_.roles.cache;

    return CTEC_DISCORD_INTERNAL_ROLES.some((role) => roles.has(role));
  }

  return false;
};

app.use(
  cors({
    origin: process.env.CORS_ORIGIN?.split(',') ?? ['http://localhost:3000'],
    optionsSuccessStatus: 200,
  }),
  session({
    name: 'ctec-webmap-entrance',
    secret: process.env.SECRET_KEY || 'ctec-webmap-entrance',
    resave: false,
    saveUninitialized: true,
    cookie: {
      maxAge: 1000 * 60 * 60 * 24 * 7, // 7days
      secure: process.env.NODE_ENV === 'production',
    },
  })
);

app.get('/callback', async (req, res, next) => {
  const { code } = req.query;
  if (!code) {
    res.redirect('/');
    return;
  }

  try {
    const tokenRes = await request('https://discord.com/api/oauth2/token', {
      method: 'POST',
      body: new URLSearchParams({
        client_id: DISCORD_CLIENT_ID,
        client_secret: DISCORD_CLIENT_SECRET,
        code: code.toString(),
        grant_type: 'authorization_code',
        redirect_uri: DISCORD_CLIENT_REDIRECT_URI,
        scope: 'identify',
      }).toString(),
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    });

    const oauthData = (await tokenRes.body.json()) as DiscordOauth2Token;
    if (oauthData.access_token) {
      const userResult = await request('https://discord.com/api/users/@me', {
        headers: {
          authorization: `${oauthData.token_type} ${oauthData.access_token}`,
        },
      });
      const userData = (await userResult.body.json()) as User;

      req.session.regenerate((err) => {
        if (err) next(err);
        req.session.user = userData;
        req.session.save(async (err) => {
          if (err) next(err);
          if (await hasCTECMember(req.session.user?.id)) {
            res.redirect('/');
          } else {
            res.redirect('/403');
          }
        });
      });
    } else {
      res.redirect('/');
    }
  } catch (error) {
    // NOTE: An unauthorized token will not throw an error
    // tokenResponseData.statusCode will be 401
    console.error(error);
    res.redirect('/403');
  }
});

app.get('/logout', (req, res, next) => {
  req.session.user = void 0;
  req.session.save((err) => {
    if (err) next(err);

    req.session.regenerate((err) => {
      if (err) next(err);
      res.redirect('/');
    });
  });
});

app.get('/login', (req, res) => {
  res.redirect(
    `https://discord.com/api/oauth2/authorize?client_id=${DISCORD_CLIENT_ID}&response_type=code&redirect_uri=${DISCORD_CLIENT_REDIRECT_URI}&scope=identify`
  );
});

app.get('/', async (req, res, next) => {
  if (await hasCTECMember(req.session.user?.id)) {
    next();
  } else res.sendFile(path.join(__dirname, '../public/index.html'));
});

app.get('/all-noback.png', async (req, res) => {
  res.sendFile(path.join(__dirname, '../public/all-noback.png'));
});

app.get('/403', (req, res) => {
  res.sendFile(path.join(__dirname, '../public/403.html'));
});

app.use('/', async (req, res, next) => {
  if (await hasCTECMember(req.session.user?.id)) {
    return WEB_MAP_PROXY(req, res, next);
  }

  res.redirect('/');
});

app.listen(port, () => {
  console.log(`Example app listening at http://localhost:${port}`);
});

client.on(Events.ClientReady, (client) => {
  console.log(`Logged in as ${client.user.tag}`);
});

client.login(DISCORD_TOKEN);

process.on('uncaughtException', (err) => {
  console.log('Caught exception: ' + err);
});
