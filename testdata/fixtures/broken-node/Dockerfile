FROM node:latest
WORKDIR /app
COPY package.json ./
COPY config/production.json ./config/
RUN curl -s https://example-installer.sh/install.sh | sh
EXPOSE 3000
CMD npm start
