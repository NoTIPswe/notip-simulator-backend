import { NestFactory } from '@nestjs/core';
import { SwaggerModule, DocumentBuilder } from '@nestjs/swagger';
import { writeFileSync } from 'fs';
import { AppModule } from '../src/app.module';

async function generateOpenApi(): Promise<void> {
  const app = await NestFactory.create(AppModule, { logger: false });

  const config = new DocumentBuilder()
    .setTitle('NoTIP Management API')
    .setDescription('NoTIP Management API OpenAPI specification')
    .setVersion(process.env.npm_package_version ?? '1.0.0')
    .build();

  const document = SwaggerModule.createDocument(app, config);
  writeFileSync(
    'openapi.json',
    JSON.stringify(document, null, 2) + '\n',
    'utf8',
  );

  await app.close();
  console.log('OpenAPI spec written to openapi.json');
}

generateOpenApi().catch((err) => {
  console.error('Failed to generate OpenAPI spec:', err);
  process.exit(1);
});
