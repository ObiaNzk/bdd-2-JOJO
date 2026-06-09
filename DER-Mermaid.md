erDiagram
  DEPORTE ||--o{ DISCIPLINA : "agrupa"
  DISCIPLINA ||--o{ EVENTO : "se compite en"
  JUEGO_OLIMPICO ||--o{ EVENTO : "incluye"
  PAIS ||--o{ JUEGO_OLIMPICO : "es anfitrión"
  JUEGO_OLIMPICO ||--o{ JUEGO_OLIM_PAIS : "tiene participantes"
  PAIS ||--o{ JUEGO_OLIM_PAIS : "participa en"
  PAIS ||--o{ ATLETA : "nacionalidad"
  JUEGO_OLIM_PAIS ||--o{ EQUIPO : "inscribe"
  EVENTO ||--o{ EQUIPO : "compite en"
  EQUIPO ||--o{ EQUIPO_ATLETA : "compuesto por"
  ATLETA ||--o{ EQUIPO_ATLETA : "integra"
  EQUIPO ||--o{ MEDALLA : "obtiene"
  EVENTO ||--o{ EVENTO : "ronda previa de"
  EVENTO ||--o{ RESULTADO_EVENTO : "registra"
  RESULTADO_EVENTO ||--o{ MARCA_RECORD : "embebe"

  PAIS {
    bigint id_pais PK
    text nombre
  }
  JUEGO_OLIMPICO {
    bigint id_juego PK
    int anio
    text ciudad
    bigint id_pais_anfitrion FK
  }
  DEPORTE {
    bigint id_deporte PK
    text nombre
  }
  DISCIPLINA {
    bigint id_disciplina PK
    bigint id_deporte FK
    text nombre
  }
  ATLETA {
    bigint id_atleta PK
    bigint id_pais FK
    text nombre
  }
  JUEGO_OLIM_PAIS {
    bigint id_jo_pais PK
    bigint id_juego FK
    bigint id_pais FK
  }
  EVENTO {
    bigint id_evento PK
    bigint id_juego FK
    bigint id_disciplina FK
    text nombre
    date fecha_evento
    text fase
    bigint id_evento_previo FK
    boolean realizado
  }
  EQUIPO {
    bigint id_equipo PK
    bigint id_jo_pais FK
    bigint id_evento FK
  }
  EQUIPO_ATLETA {
    bigint id_equipo PK "FK"
    bigint id_atleta PK "FK"
  }
  MEDALLA {
    bigint id_medalla PK
    bigint id_equipo FK
    text tipo
  }
  RESULTADO_EVENTO {
    string _id PK
    bigint id_evento
    text nombre_evento
    bigint id_juego
    text nombre_juego
    bigint id_disciplina
    text nombre_disciplina
    text deporte
    text formato
    datetime fecha
    object resultado
  }
  MARCA_RECORD {
    bigint id_atleta
    text nombre_atleta
    text tipo_record
    text metrica
    float valor
  }

