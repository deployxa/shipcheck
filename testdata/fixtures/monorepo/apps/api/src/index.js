const PORT = process.env.API_PORT || 3000;
require("fastify")().listen(PORT);
